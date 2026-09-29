import { useEffect } from 'react';

/**
 * 写入浏览器标签页标题。
 *
 * 传空值时不接管标题（例如个股名还没加载出来），
 * 让路由级的 `titleForPath` 标题继续生效。
 */
export function useDocumentTitle(title?: string) {
  useEffect(() => {
    if (title) document.title = title;
  }, [title]);
}
