package gateway

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var (
	// ErrNoGateway 表示未找到网关地址
	ErrNoGateway = errors.New("无法找到网关地址")
)

// GetDefaultGateway 返回系统的默认网关IP地址
func GetDefaultGateway() (net.IP, error) {
	switch runtime.GOOS {
	case "windows":
		return getWindowsGateway()
	case "darwin":
		return getDarwinGateway()
	case "linux":
		return getLinuxGateway()
	default:
		// 尝试通用方法
		return getGenericGateway()
	}
}

// Windows系统获取网关
func getWindowsGateway() (net.IP, error) {
	// 使用route print命令
	out, err := exec.Command("route", "print", "0.0.0.0").Output()
	if err != nil {
		return nil, fmt.Errorf("执行route命令出错: %v", err)
	}

	// 正则匹配网关IP
	pattern := regexp.MustCompile(`0.0.0.0\s+0.0.0.0\s+(\d+\.\d+\.\d+\.\d+)`)
	matches := pattern.FindStringSubmatch(string(out))
	if len(matches) > 1 {
		return net.ParseIP(matches[1]), nil
	}

	// 尝试ipconfig命令
	out, err = exec.Command("ipconfig").Output()
	if err != nil {
		return nil, fmt.Errorf("执行ipconfig命令出错: %v", err)
	}

	// 匹配"默认网关"或"Default Gateway"
	pattern = regexp.MustCompile(`(默认网关|Default Gateway)[^\d]*: (\d+\.\d+\.\d+\.\d+)`)
	matches = pattern.FindStringSubmatch(string(out))
	if len(matches) > 2 {
		return net.ParseIP(matches[2]), nil
	}

	return nil, ErrNoGateway
}

// macOS系统获取网关
func getDarwinGateway() (net.IP, error) {
	// 使用netstat -nr命令
	out, err := exec.Command("netstat", "-nr").Output()
	if err != nil {
		return nil, fmt.Errorf("执行netstat命令出错: %v", err)
	}

	// 解析netstat输出找到默认路由
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "default" {
			return net.ParseIP(fields[1]), nil
		}
	}

	// 尝试route -n get命令
	out, err = exec.Command("route", "-n", "get", "default").Output()
	if err != nil {
		return nil, fmt.Errorf("执行route命令出错: %v", err)
	}

	pattern := regexp.MustCompile(`gateway: (\d+\.\d+\.\d+\.\d+)`)
	matches := pattern.FindStringSubmatch(string(out))
	if len(matches) > 1 {
		return net.ParseIP(matches[1]), nil
	}

	return nil, ErrNoGateway
}

// Linux系统获取网关
func getLinuxGateway() (net.IP, error) {
	// 尝试读取/proc/net/route文件
	out, err := exec.Command("cat", "/proc/net/route").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for i, line := range lines {
			fields := strings.Fields(line)
			if i > 0 && len(fields) >= 3 && fields[1] == "00000000" {
				// 第三列是十六进制网关，需要转换
				hexGateway := fields[2]
				// Linux特有的小端字节序存储方式
				gateway := fmt.Sprintf("%d.%d.%d.%d",
					hexToByte(hexGateway[6:8]),
					hexToByte(hexGateway[4:6]),
					hexToByte(hexGateway[2:4]),
					hexToByte(hexGateway[0:2]))
				return net.ParseIP(gateway), nil
			}
		}
	}

	// 备用方法: 使用ip命令
	out, err = exec.Command("ip", "route", "show", "default").Output()
	if err == nil {
		pattern := regexp.MustCompile(`default via (\d+\.\d+\.\d+\.\d+)`)
		matches := pattern.FindStringSubmatch(string(out))
		if len(matches) > 1 {
			return net.ParseIP(matches[1]), nil
		}
	}

	// 再次备用: 使用route命令
	out, err = exec.Command("route", "-n").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "0.0.0.0" {
				return net.ParseIP(fields[1]), nil
			}
		}
	}

	return nil, ErrNoGateway
}

// 通用方法尝试推断网关
func getGenericGateway() (net.IP, error) {
	// 获取所有接口
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("无法获取网络接口: %v", err)
	}

	// 检查每个接口
	for _, iface := range interfaces {
		// 排除回环和非活动接口
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}

			ip := ipnet.IP.To4()
			// 根据IP和掩码猜测网关 (通常是x.x.x.1)
			return guessGatewayFromIP(ip, ipnet.Mask), nil
		}
	}

	return nil, ErrNoGateway
}

// 从IP和掩码猜测网关地址
func guessGatewayFromIP(ip net.IP, mask net.IPMask) net.IP {
	// 最常见的是网络地址的第一个可用IP，如192.168.1.1
	if len(ip) == 4 && len(mask) == 4 {
		// 计算网络地址
		network := make(net.IP, 4)
		for i := 0; i < 4; i++ {
			network[i] = ip[i] & mask[i]
		}

		// 常见的几种情况
		// 1. 网关通常是网络地址的第一个地址 (x.x.x.1)
		gateway1 := make(net.IP, 4)
		copy(gateway1, network)
		gateway1[3] = 1

		// 2. 有时网关是第二个地址 (x.x.x.2)
		gateway2 := make(net.IP, 4)
		copy(gateway2, network)
		gateway2[3] = 2

		// 3. 有些特殊情况 (x.x.x.254)
		gateway3 := make(net.IP, 4)
		copy(gateway3, network)
		gateway3[3] = 254

		// 优先返回最可能的网关
		return gateway1
	}

	return nil
}

// 将十六进制字符串转换为字节值
func hexToByte(hex string) byte {
	// 简单的十六进制转换
	var val byte
	fmt.Sscanf(hex, "%02x", &val)
	return val
}

// 额外工具函数: 获取所有可能的网关
func GetAllPossibleGateways() []net.IP {
	var gateways []net.IP

	// 获取所有接口
	interfaces, err := net.Interfaces()
	if err != nil {
		return gateways
	}

	// 检查每个接口
	for _, iface := range interfaces {
		// 排除回环和非活动接口
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}

			ip := ipnet.IP.To4()
			mask := ipnet.Mask

			// 增加常见的网关地址
			if len(ip) == 4 && len(mask) == 4 {
				// 计算网络地址
				network := make(net.IP, 4)
				for i := 0; i < 4; i++ {
					network[i] = ip[i] & mask[i]
				}

				// 常见的网关地址: .1
				gateway1 := make(net.IP, 4)
				copy(gateway1, network)
				gateway1[3] = 1
				gateways = append(gateways, gateway1)

				// 常见的网关地址: .254
				gateway2 := make(net.IP, 4)
				copy(gateway2, network)
				gateway2[3] = 254
				gateways = append(gateways, gateway2)

				// 常见的网关地址: .2
				gateway3 := make(net.IP, 4)
				copy(gateway3, network)
				gateway3[3] = 2
				gateways = append(gateways, gateway3)
			}
		}
	}

	return gateways
}

// 检查特定IP是否可能是网关
func CheckIfGateway(ip net.IP) bool {
	if ip == nil {
		return false
	}

	// 执行ping测试
	cmd := exec.Command("ping", "-c", "1", "-W", "1", ip.String())
	err := cmd.Run()
	if err != nil {
		return false
	}

	// 尝试获取端口22,23,80,443,8080等常见端口的连接
	ports := []string{"80", "443", "8080", "53"}
	for _, port := range ports {
		conn, err := net.DialTimeout("tcp", ip.String()+":"+port, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
	}

	return true // 如果能ping通，假定是网关
}
