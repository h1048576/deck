(async () => {
  const failure = error => ({ items: [], error });
  const root = document.getElementById('root');
  if (window.top !== window || !/^https:\/\/127\.0\.0\.1:\d+$/.test(location.origin) ||
      window.__APP_CONFIG__?.productName !== 'antigravity' ||
      typeof window.nativeStorage?.updateItems !== 'function' || !window.electronNative || !root) return null;

  const findService = () => {
    const trigger = root.querySelector('[data-testid="model-selector-trigger"]');
    for (const element of [trigger, root.firstElementChild]) {
      if (!element) continue;
      const key = Object.keys(element).find(name => name.startsWith('__reactFiber$'));
      for (let fiber = element[key]; fiber; fiber = fiber.return) {
        const provider = fiber.memoizedProps?.value?.core?.cloudCodeService;
        if (typeof provider?.retrieveUserQuotaSummary === 'function') return provider;
        for (let context = fiber.dependencies?.firstContext; context; context = context.next) {
          const service = context.memoizedValue?.core?.cloudCodeService;
          if (typeof service?.retrieveUserQuotaSummary === 'function') return service;
        }
      }
    }
    return null;
  };

  const service = findService();
  if (!service) return failure('尚未读取到 Antigravity 用量，请登录并打开主界面后刷新');
  let groups;
  try {
    const result = await service.retrieveUserQuotaSummary(true);
    groups = result?.response?.groups;
  } catch {
    return failure('Antigravity 用量查询失败，请检查网络或登录状态后刷新');
  }

  const windowLabel = bucket => {
    const window = bucket.window?.trim().toLowerCase();
    if (window === 'daily') return '每日';
    if (window === 'weekly') return '每周';
    const match = /^(\d+)([smhd])$/.exec(window);
    const units = { s: '秒', m: '分钟', h: '小时', d: '天' };
    return match ? `${match[1]} ${units[match[2]]}` : bucket.displayName?.trim() || '';
  };
  const formatReset = timestamp => {
    if (!timestamp || timestamp.seconds == null) return '';
    const date = new Date(Number(timestamp.seconds) * 1000 + Number(timestamp.nanos ?? 0) / 1e6);
    if (!Number.isFinite(date.getTime())) return '';
    return date.toLocaleString('zh-CN', {
      year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23',
    });
  };
  const items = [];
  for (const group of groups ?? []) {
    for (const bucket of group.buckets ?? []) {
      if (bucket.disabled) continue;
      const fraction = bucket.remaining?.case === 'remainingFraction' ? bucket.remaining.value : null;
      const used = Number.isFinite(fraction) && fraction >= 0 && fraction <= 1 ? Math.round((1 - fraction) * 100) : null;
      const reset = formatReset(bucket.resetTime);
      if (used === null && !reset) continue;
      const title = group.displayName?.trim();
      const window = windowLabel(bucket);
      const label = [title, window === title ? '' : window].filter(Boolean).join(' · ') || '模型配额';
      items.push({ label, used, reset, bucketId: bucket.bucketId });
    }
  }
  return items.length ? { items, error: '' } : failure('当前 Antigravity 账号暂无模型用量数据');
})()
