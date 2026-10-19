import {parseJSON, MAX_CURRENT_BYTES, readBounded, validateCurrent} from '/shared/contracts.mjs';
const $ = id => document.getElementById(id);
let generation = 0, currentDispatch = '';
function element(tag, text, parent) { const e = document.createElement(tag); e.textContent = text; parent.append(e); return e; }
async function load() {
  const serial = ++generation;
  $('platforms').replaceChildren(); $('servers').replaceChildren(); $('release').textContent = '';
  $('dispatch').textContent = '尚未发布'; $('copy-dispatch').disabled = true; currentDispatch = '';
  $('status').textContent = '正在读取已发布版本…';
  try {
    const r = await fetch('/release.json', {cache:'no-store',redirect:'error'});
    const release = validateCurrent(parseJSON(new TextDecoder().decode(await readBounded(r, MAX_CURRENT_BYTES))));
    if (serial !== generation) return;
    $('release').textContent = `发布版本：${release.releaseId}`;
    $('status').textContent = '下载链接与资源清单来自同一份已发布配置。';
    currentDispatch = release.defaultDispatchUrl; $('dispatch').textContent = currentDispatch; $('copy-dispatch').disabled = false;
    for (const [platform, name] of [['android','Android'], ['macos','macOS'], ['windows','Windows']]) {
      const section = element('section', '', $('platforms')); section.className = 'tool-panel'; element('h2', name, section);
      const client = release.dispatch.update?.clients?.[platform];
      if (!client) { element('p', '尚未发布安装包', section); continue; }
      element('p', `版本 ${client.version}${client.minVersion ? ` · 最低版本 ${client.minVersion}` : ''}`, section);
      const anchor = element('a', `下载 ${name}`, section); anchor.href = client.artifactUrl; anchor.className = 'download-link'; anchor.rel = 'noopener noreferrer';
      const details = element('dl', '', section);
      element('dt','文件大小',details); element('dd',`${client.sizeBytes.toLocaleString('zh-CN')} 字节`,details);
      element('dt','SHA-256',details); element('dd',client.sha256,details);
      const compatible = release.dispatch.update.resourceManifests?.[platform]?.[client.version];
      element('p', compatible ? '已发布对应版本资源清单' : '未声明精确版本资源映射；请先确认兼容性', section);
    }
    for (const s of release.dispatch.servers) element('li', `${s.name} · ${s.zone} · ${s.host}:${s.port}`, $('servers'));
    if (!release.dispatch.servers.length) element('li', '未发布游戏服务器', $('servers'));
  } catch (e) {
    if (serial !== generation) return;
    $('status').textContent = '尚无可用的已验证发布，或配置读取失败。请稍后重试。';
  }
}
$('retry').onclick = load;
$('copy-dispatch').onclick = async () => {
  if (!currentDispatch) return;
  try { await navigator.clipboard.writeText(currentDispatch); $('status').textContent = '已复制 dispatch 地址'; }
  catch { $('status').textContent = '无法自动复制，请手动选择上方地址。'; }
};
load();
