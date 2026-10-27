#!/usr/bin/env node
/**
 * 生成社交分享图 site/public/og.png（1200×630）。
 *
 * 为什么用脚本而不是手工图：文案与配色跟落地页共享同一套变量，
 * 改标题时重新跑一次就行，不用再找设计软件。
 *
 * 依赖 sharp（devDependency，Astro 自身也用它做图像优化）。
 * 用法：node scripts/gen-og.mjs
 */

import { writeFileSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import sharp from 'sharp';

const HERE = dirname(fileURLToPath(import.meta.url));
const SITE = resolve(HERE, '..');
const OUT = resolve(SITE, 'public/og.png');

const WIDTH = 1200;
const HEIGHT = 630;

const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${WIDTH}" height="${HEIGHT}" viewBox="0 0 ${WIDTH} ${HEIGHT}">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0%" stop-color="#0b1220"/>
      <stop offset="100%" stop-color="#121c2e"/>
    </linearGradient>
    <radialGradient id="glow" cx="18%" cy="12%" r="70%">
      <stop offset="0%" stop-color="#2f6fed" stop-opacity="0.42"/>
      <stop offset="100%" stop-color="#2f6fed" stop-opacity="0"/>
    </radialGradient>
    <pattern id="grid" width="48" height="48" patternUnits="userSpaceOnUse">
      <path d="M48 0H0V48" fill="none" stroke="#ffffff" stroke-opacity="0.04" stroke-width="1"/>
    </pattern>
  </defs>

  <rect width="${WIDTH}" height="${HEIGHT}" fill="url(#bg)"/>
  <rect width="${WIDTH}" height="${HEIGHT}" fill="url(#grid)"/>
  <rect width="${WIDTH}" height="${HEIGHT}" fill="url(#glow)"/>

  <text x="80" y="150" font-family="PingFang SC, Hiragino Sans GB, Microsoft YaHei, sans-serif"
        font-size="26" fill="#93a1b8" letter-spacing="4">自托管 · MIT · Go + React</text>

  <text x="80" y="268" font-family="PingFang SC, Hiragino Sans GB, Microsoft YaHei, sans-serif"
        font-size="78" font-weight="700" fill="#e8edf6">今天买什么，</text>
  <text x="80" y="368" font-family="PingFang SC, Hiragino Sans GB, Microsoft YaHei, sans-serif"
        font-size="78" font-weight="700" fill="#e8edf6">什么时候卖</text>

  <text x="80" y="452" font-family="PingFang SC, Hiragino Sans GB, Microsoft YaHei, sans-serif"
        font-size="28" fill="#b9c6da">AI 提出规律，确定性引擎用真实 A 股数据验证；</text>
  <text x="80" y="496" font-family="PingFang SC, Hiragino Sans GB, Microsoft YaHei, sans-serif"
        font-size="28" fill="#b9c6da">证据不足就拒绝推荐。</text>

  <g transform="translate(80, 540)">
    <rect width="230" height="52" rx="10" fill="#2f6fed"/>
    <text x="115" y="34" text-anchor="middle" font-family="PingFang SC, Hiragino Sans GB, sans-serif"
          font-size="22" font-weight="600" fill="#ffffff">进入控制台</text>
    <text x="262" y="34" font-family="ui-monospace, SFMono-Regular, Menlo, monospace"
          font-size="22" fill="#7fa8f0">./tongstock serve</text>
  </g>

  <text x="1120" y="576" text-anchor="end" font-family="PingFang SC, Hiragino Sans GB, sans-serif"
        font-size="24" font-weight="700" fill="#e8edf6">TongStock</text>
</svg>`;

mkdirSync(dirname(OUT), { recursive: true });
const buffer = await sharp(Buffer.from(svg)).png({ compressionLevel: 9 }).toBuffer();
writeFileSync(OUT, buffer);
console.log(`[gen-og] 写入 public/og.png（${WIDTH}×${HEIGHT}，${(buffer.length / 1024).toFixed(1)} KiB）`);
