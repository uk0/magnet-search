// 复制磁力链接
document.addEventListener('DOMContentLoaded', function() {
    // 点击磁力链接时复制到剪贴板
    document.querySelectorAll('.magnet-link').forEach(link => {
        link.addEventListener('click', function(e) {
            e.preventDefault();
            const magnetUrl = this.getAttribute('href');

            navigator.clipboard.writeText(magnetUrl).then(() => {
                // 创建一个临时提示
                const toast = document.createElement('div');
                toast.textContent = '磁力链接已复制到剪贴板';
                toast.style.cssText = `
                    position: fixed;
                    bottom: 20px;
                    left: 50%;
                    transform: translateX(-50%);
                    background-color: rgba(0, 0, 0, 0.8);
                    color: white;
                    padding: 10px 20px;
                    border-radius: 4px;
                    z-index: 10000;
                `;
                document.body.appendChild(toast);

                // 3秒后移除提示
                setTimeout(() => {
                    toast.remove();
                }, 3000);

                // 可以选择性地在一个新标签页中打开磁力链接
                window.open(magnetUrl, '_blank');
            }).catch(err => {
                console.error('复制失败:', err);
                // 复制失败时直接打开链接
                window.open(magnetUrl, '_blank');
            });
        });
    });
});

// 更新搜索过滤器
function updateFilter(key, value) {
    const url = new URL(window.location.href);

    if (value) {
        url.searchParams.set(key, value);
    } else {
        url.searchParams.delete(key);
    }

    // 重置页码
    url.searchParams.set('page', '1');

    window.location.href = url.toString();
}

// 切换到指定页码
function goToPage(page) {
    const url = new URL(window.location.href);
    url.searchParams.set('page', page);
    window.location.href = url.toString();
}

// static/js/main.js
document.addEventListener('DOMContentLoaded', function() {
    // 点击磁力链接时复制到剪贴板
    document.querySelectorAll('.magnet-link').forEach(link => {
        link.addEventListener('click', function(e) {
            e.preventDefault();
            const magnetUrl = this.getAttribute('href');

            navigator.clipboard.writeText(magnetUrl).then(() => {
                // 创建一个临时提示
                const toast = document.createElement('div');
                toast.textContent = '磁力链接已复制到剪贴板';
                toast.style.cssText = `
                    position: fixed;
                    bottom: 20px;
                    left: 50%;
                    transform: translateX(-50%);
                    background-color: rgba(0, 0, 0, 0.8);
                    color: white;
                    padding: 10px 20px;
                    border-radius: 4px;
                    z-index: 10000;
                `;
                document.body.appendChild(toast);

                // 3秒后移除提示
                setTimeout(() => {
                    toast.remove();
                }, 3000);
            }).catch(err => {
                console.error('复制失败:', err);
                window.open(magnetUrl, '_blank');
            });
        });
    });

    // 实现淡入动画效果
    document.querySelectorAll('.torrent-item').forEach((item, index) => {
        item.style.opacity = '0';
        item.style.transform = 'translateY(20px)';
        item.style.transition = 'opacity 0.3s ease, transform 0.3s ease';

        setTimeout(() => {
            item.style.opacity = '1';
            item.style.transform = 'translateY(0)';
        }, index * 50);
    });
});