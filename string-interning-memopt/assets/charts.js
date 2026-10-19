// 图表初始化：所有颜色取自 CSS 变量
(function () {
  var style = getComputedStyle(document.documentElement);
  var accent = style.getPropertyValue('--accent').trim();
  var accent2 = style.getPropertyValue('--accent2').trim();
  var ink = style.getPropertyValue('--ink').trim();
  var muted = style.getPropertyValue('--muted').trim();
  var rule = style.getPropertyValue('--rule').trim();
  var bg2 = style.getPropertyValue('--bg2').trim();

  var FONT = "'Work Sans', 'PingFang SC', 'Microsoft YaHei', sans-serif";

  function baseTooltip() {
    return { trigger: 'axis', appendToBody: true, axisPointer: { type: 'shadow' } };
  }
  function axisCommon() {
    return {
      axisLine: { lineStyle: { color: rule } },
      axisLabel: { color: muted, fontFamily: FONT },
      splitLine: { lineStyle: { color: rule } }
    };
  }

  // ---------- 图 1：300 万唯一短串峰值 RSS（对数轴） ----------
  var c1 = echarts.init(document.getElementById('chart-rss'), null, { renderer: 'svg' });
  c1.setOption({
    animation: false,
    tooltip: { trigger: 'axis', appendToBody: true, axisPointer: { type: 'shadow' },
      formatter: function (ps) { return ps[0].name + '<br/>峰值 RSS：<b>' + ps[0].value + ' MB</b>'; } },
    grid: { left: 70, right: 30, top: 30, bottom: 40 },
    xAxis: Object.assign(axisCommon(), {
      type: 'category',
      data: ['PUC-Lua 5.4.9', 'A · 永久驻留（改前）', 'B · 两代缓存', 'C · 弱引用（选定）'],
      axisLabel: { color: ink, fontFamily: FONT, interval: 0 }
    }),
    yAxis: Object.assign(axisCommon(), {
      type: 'log', logBase: 10, name: 'MB',
      nameTextStyle: { color: muted, fontFamily: FONT }
    }),
    series: [{
      type: 'bar',
      barWidth: '52%',
      data: [
        { value: 1.9, itemStyle: { color: muted } },
        { value: 609.4, itemStyle: { color: accent2 } },
        { value: 17.5, itemStyle: { color: accent + '99' } },
        { value: 21.0, itemStyle: { color: accent } }
      ],
      label: { show: true, position: 'top', color: ink, fontFamily: FONT,
        formatter: function (p) { return p.value + ' MB'; } }
    }]
  });
  window.addEventListener('resize', function () { c1.resize(); });

  // ---------- 图 2：唯一短串 wall time 改前/改后/PUC ----------
  var c2 = echarts.init(document.getElementById('chart-wall'), null, { renderer: 'svg' });
  c2.setOption({
    animation: false,
    tooltip: baseTooltip(),
    legend: { bottom: 0, textStyle: { color: ink, fontFamily: FONT } },
    grid: { left: 60, right: 30, top: 30, bottom: 60 },
    xAxis: Object.assign(axisCommon(), {
      type: 'category',
      data: ['200 万唯一短串', '400 万唯一短串'],
      axisLabel: { color: ink, fontFamily: FONT }
    }),
    yAxis: Object.assign(axisCommon(), {
      type: 'value', name: '秒',
      nameTextStyle: { color: muted, fontFamily: FONT }
    }),
    series: [
      { name: 'PUC-Lua 5.4.9', type: 'bar', data: [0.37, 0.77], itemStyle: { color: muted },
        label: { show: true, position: 'top', color: ink, fontFamily: FONT, formatter: '{c} s' } },
      { name: '改前（永久驻留）', type: 'bar', data: [3.08, 9.35], itemStyle: { color: accent2 },
        label: { show: true, position: 'top', color: ink, fontFamily: FONT, formatter: '{c} s' } },
      { name: '改后（弱引用驻留）', type: 'bar', data: [2.14, 4.40], itemStyle: { color: accent },
        label: { show: true, position: 'top', color: ink, fontFamily: FONT, formatter: '{c} s' } }
    ]
  });
  window.addEventListener('resize', function () { c2.resize(); });

  // ---------- 图 3：唯一短串峰值 RSS 改前/改后/PUC ----------
  var c3 = echarts.init(document.getElementById('chart-rss2'), null, { renderer: 'svg' });
  c3.setOption({
    animation: false,
    tooltip: baseTooltip(),
    legend: { bottom: 0, textStyle: { color: ink, fontFamily: FONT } },
    grid: { left: 70, right: 30, top: 30, bottom: 60 },
    xAxis: Object.assign(axisCommon(), {
      type: 'category',
      data: ['200 万唯一短串', '400 万唯一短串'],
      axisLabel: { color: ink, fontFamily: FONT }
    }),
    yAxis: Object.assign(axisCommon(), {
      type: 'value', name: 'MB',
      nameTextStyle: { color: muted, fontFamily: FONT }
    }),
    series: [
      { name: 'PUC-Lua 5.4.9', type: 'bar', data: [1.9, 1.9], itemStyle: { color: muted },
        label: { show: true, position: 'top', color: ink, fontFamily: FONT, formatter: '{c} MB' } },
      { name: '改前（永久驻留）', type: 'bar', data: [487.9, 936.4], itemStyle: { color: accent2 },
        label: { show: true, position: 'top', color: ink, fontFamily: FONT, formatter: '{c} MB' } },
      { name: '改后（弱引用驻留）', type: 'bar', data: [21.9, 21.7], itemStyle: { color: accent },
        label: { show: true, position: 'top', color: ink, fontFamily: FONT, formatter: '{c} MB' } }
    ]
  });
  window.addEventListener('resize', function () { c3.resize(); });

  // ---------- 图 4：50 万唯一短串 collect 前后 ----------
  var c4 = echarts.init(document.getElementById('chart-reclaim'), null, { renderer: 'svg' });
  c4.setOption({
    animation: false,
    tooltip: baseTooltip(),
    legend: { bottom: 0, textStyle: { color: ink, fontFamily: FONT } },
    grid: { left: 70, right: 30, top: 30, bottom: 60 },
    xAxis: Object.assign(axisCommon(), {
      type: 'category',
      data: ['PUC-Lua 5.4.9', '改前（永久驻留）', '改后（弱引用驻留）'],
      axisLabel: { color: ink, fontFamily: FONT, interval: 0 }
    }),
    yAxis: Object.assign(axisCommon(), {
      type: 'value', name: 'MB（存活）',
      nameTextStyle: { color: muted, fontFamily: FONT }
    }),
    series: [
      { name: '压入 50 万串后', type: 'bar', data: [47.7, 147.2, null], itemStyle: { color: accent + '99' },
        label: { show: true, position: 'top', color: ink, fontFamily: FONT,
          formatter: function (p) { return p.value == null ? '未单列' : p.value + ' MB'; } } },
      { name: 'collectgarbage 后', type: 'bar', data: [4.0, 118.1, 53.8], itemStyle: { color: accent2 },
        label: { show: true, position: 'top', color: ink, fontFamily: FONT, formatter: '{c} MB' } }
    ]
  });
  window.addEventListener('resize', function () { c4.resize(); });

  // ---------- 图 5：基准归一化（永久 = 1.00） ----------
  var c5 = echarts.init(document.getElementById('chart-bench'), null, { renderer: 'svg' });
  c5.setOption({
    animation: false,
    tooltip: baseTooltip(),
    legend: { bottom: 0, textStyle: { color: ink, fontFamily: FONT } },
    grid: { left: 150, right: 60, top: 30, bottom: 60 },
    xAxis: Object.assign(axisCommon(), {
      type: 'value', name: '相对耗时',
      nameTextStyle: { color: muted, fontFamily: FONT }
    }),
    yAxis: Object.assign(axisCommon(), {
      type: 'category',
      data: ['StringKeyTable', 'StringOps', 'TableSort', 'Compile'],
      axisLabel: { color: ink, fontFamily: FONT }
    }),
    series: [
      { name: 'A · 永久（=1.00）', type: 'bar', data: [1.00, 1.00, 1.00, 1.00], itemStyle: { color: accent2 },
        label: { show: true, position: 'right', color: muted, fontFamily: FONT, formatter: '1.00' } },
      { name: 'B · 两代缓存', type: 'bar', data: [0.68, 0.70, 0.93, 0.87], itemStyle: { color: muted },
        label: { show: true, position: 'right', color: ink, fontFamily: FONT } },
      { name: 'C · 弱引用（选定）', type: 'bar', data: [2.00, 0.87, 0.97, 1.01], itemStyle: { color: accent },
        label: { show: true, position: 'right', color: ink, fontFamily: FONT } }
    ]
  });
  window.addEventListener('resize', function () { c5.resize(); });
})();
