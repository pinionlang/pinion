const SYL = ["ka", "lo", "mi", "nu", "pe", "ra", "si", "to", "ve", "wa", "xi", "yo", "ze", "bu", "da", "fo"];

let x = 42;
const parts: string[] = [];
for (let i = 0; i < 300_000; i++) {
  x = (x * 48271) % 2147483647;
  parts.push(SYL[x % 16] + SYL[Math.floor(x / 16) % 16]);
}
const text = parts.join(" ");

const counts = new Map<string, number>();
for (const w of text.split(/\s+/)) {
  if (w !== "") counts.set(w, (counts.get(w) ?? 0) + 1);
}
const ranked = [...counts].sort((a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0));
console.log(`distinct = ${ranked.length}`);
for (const [w, c] of ranked.slice(0, 10)) {
  console.log(`${w} ${c}`);
}
