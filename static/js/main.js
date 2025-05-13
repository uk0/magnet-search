// 更新过滤器
function updateFilter(name, value) {
    const urlParams = new URLSearchParams(window.location.search);

    // 更新对应的参数
    if (value) {
        urlParams.set(name, value);
    } else {
        urlParams.delete(name);
    }

    // 重置页码
    if (name !== 'page') {
        urlParams.delete('page');
    }

    // 重定向到新URL
    window.location.href = window.location.pathname + '?' + urlParams.toString();
}

// 复制磁力链接和文件列表展开/收起功能
document.addEventListener('DOMContentLoaded', function () {
    // 复制磁力链接功能
    const copyButtons = document.querySelectorAll('.copy-magnet');
    copyButtons.forEach(button => {
        button.addEventListener('click', function () {
            const magnetLink = this.getAttribute('data-magnetlink');

            // 检查是否支持clipboard API
            if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(magnetLink)
                    .then(() => {
                        showCopySuccess(this);
                    })
                    .catch(err => {
                        console.error('剪贴板API失败:', err);
                        fallbackCopy(magnetLink, this);
                    });
            } else {
                // 使用备用复制方法
                fallbackCopy(magnetLink, this);
            }
        });
    });

    // 显示复制成功提示
    function showCopySuccess(button) {
        const originalText = button.innerHTML;
        button.innerHTML = '<i class="icon-check"></i> 已复制';
        button.classList.add('copied');

        // 3秒后恢复原状
        setTimeout(() => {
            button.innerHTML = originalText;
            button.classList.remove('copied');
        }, 3000);
    }

    // 备用复制方法（当Clipboard API不可用时）
    function fallbackCopy(text, button) {
        try {
            // 创建一个临时的文本区域
            const textArea = document.createElement('textarea');
            textArea.value = text;

            // 确保文本区域在视图之外，但仍可选择
            textArea.style.position = 'fixed';
            textArea.style.left = '-9999px';
            textArea.style.top = '0';

            document.body.appendChild(textArea);
            textArea.focus();
            textArea.select();

            // 尝试复制文本
            const successful = document.execCommand('copy');
            document.body.removeChild(textArea);

            if (successful) {
                showCopySuccess(button);
            } else {
                alert('复制失败，请手动复制以下链接:\n' + text);
            }
        } catch (err) {
            console.error('复制失败:', err);
            alert('复制失败，请手动复制以下链接:\n' + text);
        }
    }

    // 展开/收起文件列表功能
    const toggleButtons = document.querySelectorAll('.toggle-files');
    toggleButtons.forEach(button => {
        button.addEventListener('click', function () {
            const fileListId = this.getAttribute('data-id');
            const fileList = document.getElementById(fileListId);
            const showText = this.querySelector('.show-text');
            const hideText = this.querySelector('.hide-text');

            if (fileList.style.display === 'none') {
                // 展开文件列表
                fileList.style.display = 'block';
                showText.style.display = 'none';
                hideText.style.display = 'inline';
                this.classList.add('expanded');
            } else {
                // 收起文件列表
                fileList.style.display = 'none';
                showText.style.display = 'inline';
                hideText.style.display = 'none';
                this.classList.remove('expanded');
            }
        });
    });
});