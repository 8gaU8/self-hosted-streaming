const INTERVAL_KEY = "landing-refresh-interval";
const intervalSelect = document.querySelector("#interval");
const lastUpdatedEl = document.querySelector("#last-updated");

intervalSelect.value = localStorage.getItem(INTERVAL_KEY) || "0";

function formatBytes(bytes) {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / 1024 ** i).toFixed(1)} ${units[i]}`;
}

// A ring donut as an inline SVG: a background circle plus a foreground
// arc drawn via stroke-dasharray on the circle's circumference.
function ringSvg(percent, className, label, valueText, detailText) {
  const r = 24;
  const circumference = 2 * Math.PI * r;
  const clamped = Math.max(0, Math.min(percent, 100));
  const len = (clamped / 100) * circumference;
  const dash = `${len} ${circumference - len}`;

  return `
    <div class="metric">
      <svg viewBox="0 0 60 60">
        <circle class="metric-ring-bg" cx="30" cy="30" r="${r}"></circle>
        <circle class="metric-ring ${className}" cx="30" cy="30" r="${r}" stroke-dasharray="${dash}"></circle>
      </svg>
      <div class="metric-value">${valueText}</div>
      <div class="metric-label">${label}</div>
      ${detailText ? `<div class="metric-detail">${detailText}</div>` : ""}
    </div>
  `;
}

// Two labeled horizontal bars (e.g. disk read/write, network rx/tx) scaled
// against the largest figure of that kind on the page, so relative I/O
// across cards is visible at a glance -- a read/write or rx/tx split isn't
// naturally a part-of-whole ratio, which is what made an earlier pie chart
// hard to read. Shared by disk and network usage, which only differ in
// title/labels/colors.
function ioBars(title, entries, maxBytes) {
  const widthFor = (v) => {
    if (!maxBytes || v <= 0) return 0;
    return Math.max((v / maxBytes) * 100, 3);
  };
  const total = entries.reduce((sum, e) => sum + e.value, 0);

  const rows = entries
    .map(
      ({ tag, cls, value }) => `
      <div class="io-row">
        <span class="io-tag">${tag}</span>
        <div class="io-track"><div class="io-fill ${cls}" style="width:${widthFor(value)}%"></div></div>
        <span class="io-amount">${formatBytes(value)}</span>
      </div>
    `
    )
    .join("");

  return `
    <div class="io-block">
      <div class="io-header">
        <span class="metric-label">${title}</span>
        <span class="metric-value">${formatBytes(total)}</span>
      </div>
      ${rows}
    </div>
  `;
}

function renderUsage(svc, maxDiskBytes, maxNetBytes) {
  const container = document.querySelector(`#metrics-${svc.key}`);
  if (!container) return;

  if (!svc.available) {
    container.innerHTML = `<div class="unavailable">利用不可</div>`;
    return;
  }

  const cpuRing = ringSvg(svc.cpu_percent, "cpu", "CPU", `${svc.cpu_percent}%`);
  const memRing =
    svc.mem_percent === null
      ? ringSvg(0, "mem", "メモリ", "—", formatBytes(svc.mem_used))
      : ringSvg(svc.mem_percent, "mem", "メモリ", `${svc.mem_percent}%`, formatBytes(svc.mem_used));

  const diskIO = ioBars(
    "ディスク I/O",
    [
      { tag: "読", cls: "read", value: svc.disk_read },
      { tag: "書", cls: "write", value: svc.disk_write },
    ],
    maxDiskBytes
  );
  const netIO = ioBars(
    "ネットワーク I/O",
    [
      { tag: "受", cls: "rx", value: svc.net_rx },
      { tag: "送", cls: "tx", value: svc.net_tx },
    ],
    maxNetBytes
  );

  container.innerHTML = `
    <div class="donuts">${cpuRing}${memRing}</div>
    ${diskIO}
    ${netIO}
  `;
}

async function refreshStatus() {
  try {
    const res = await fetch("/api/status");
    const data = await res.json();
    data.forEach((svc) => {
      const dot = document.querySelector(`#dot-${svc.key}`);
      if (dot) {
        dot.classList.toggle("up", svc.up);
        dot.classList.toggle("down", !svc.up);
      }
    });
  } catch (e) {
    console.error("status fetch failed", e);
  }
}

async function refreshUsage() {
  try {
    const res = await fetch("/api/usage");
    const data = await res.json();

    const diskValues = [data.total.disk_read, data.total.disk_write];
    const netValues = [data.total.net_rx, data.total.net_tx];
    data.services.forEach((svc) => {
      if (svc.available) {
        diskValues.push(svc.disk_read, svc.disk_write);
        netValues.push(svc.net_rx, svc.net_tx);
      }
    });
    const maxDiskBytes = Math.max(1, ...diskValues);
    const maxNetBytes = Math.max(1, ...netValues);

    const totalSub = document.querySelector("#total-sub");
    if (totalSub) {
      totalSub.textContent = `${data.total.container_count} コンテナ`;
    }
    renderUsage({ key: "total", available: true, ...data.total }, maxDiskBytes, maxNetBytes);
    data.services.forEach((svc) => renderUsage(svc, maxDiskBytes, maxNetBytes));

    lastUpdatedEl.textContent = `最終更新: ${new Date(data.updated_at).toLocaleTimeString()}`;
  } catch (e) {
    console.error("usage fetch failed", e);
    lastUpdatedEl.textContent = "更新に失敗しました";
  }
}

function refreshAll() {
  refreshStatus();
  refreshUsage();
}

let timer = null;
function scheduleRefresh() {
  if (timer) clearInterval(timer);
  const ms = Number(intervalSelect.value);
  timer = ms > 0 ? setInterval(refreshAll, ms) : null;
}

intervalSelect.addEventListener("change", () => {
  localStorage.setItem(INTERVAL_KEY, intervalSelect.value);
  scheduleRefresh();
});

document.querySelector("#refresh-now").addEventListener("click", refreshAll);

refreshAll();
scheduleRefresh();
