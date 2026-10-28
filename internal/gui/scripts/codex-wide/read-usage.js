(async () => {
  const failure = error => ({ items: [], error });
  let query = null;
  for (const element of document.querySelectorAll('[data-codex-intelligence-trigger]')) {
    const fiberKey = Object.keys(element).find(name => name.startsWith('__reactFiber'));
    for (let fiber = element[fiberKey]; fiber; fiber = fiber.return) {
      const props = fiber.memoizedProps;
      const client = props?.client ?? props?.queryClient;
      if (typeof client?.getQueryCache !== 'function') continue;
      query = client.getQueryCache().find({ queryKey: ['rate-limit-status'], exact: true });
      if (query) break;
    }
    if (query) break;
  }
  if (!query) return failure('尚未读取到 Codex 用量，请登录并打开一个聊天后刷新');
  try {
    if (query.state.status === 'error' || !query.state.data || Date.now() - query.state.dataUpdatedAt >= 60000) {
      await query.fetch();
    }
  } catch {
    return failure('Codex 用量查询失败，请检查网络或登录状态后刷新');
  }
  const limit = query.state.data?.rate_limit;
  if (!limit) return failure('当前 Codex 账号暂无用量数据，请检查登录状态或账号类型');
  const label = (window, index) => {
    const seconds = window.limit_window_seconds;
    if (!Number.isFinite(seconds) || seconds <= 0) return index === 0 ? '主要周期' : '次要周期';
    if (seconds % 604800 === 0) return `${seconds / 604800} 周`;
    if (seconds % 86400 === 0) return `${seconds / 86400} 天`;
    if (seconds % 3600 === 0) return `${seconds / 3600} 小时`;
    return `${Math.round(seconds / 60)} 分钟`;
  };
  const items = [limit.primary_window, limit.secondary_window].flatMap((window, index) => {
    if (!window) return [];
    const date = Number.isFinite(window.reset_at) ? new Date(window.reset_at * 1000) : null;
    return [{
      label: label(window, index),
      used: Number.isFinite(window.used_percent) ? Math.round(window.used_percent) : null,
      reset: date && Number.isFinite(date.getTime()) ? date.toLocaleString('zh-CN', {
        year: 'numeric', month: '2-digit', day: '2-digit',
        hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
      }) : '—',
    }];
  });
  return { items, error: items.length ? '' : '当前 Codex 账号暂无用量数据' };
})();
