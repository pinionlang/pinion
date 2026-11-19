function checksum(xs: number[]): number {
  let h = 0;
  for (const v of xs) h = (h * 31 + v) % 1_000_000_007;
  return h;
}

let x = 12345;
const nums: number[] = [];
const recs: { key: number; id: number }[] = [];
for (let i = 0; i < 200_000; i++) {
  x = (x * 48271) % 2147483647;
  nums.push(x % 1_000_000);
  recs.push({ key: x % 1000, id: i });
}
const sorted = Float64Array.from(nums).sort();
console.log(`ints: min = ${sorted[0]} max = ${sorted[sorted.length - 1]} checksum = ${checksum([...sorted])}`);
const byKey = [...recs].sort((a, b) => a.key - b.key);
console.log(`recs: checksum = ${checksum(byKey.map((r) => r.id))}`);
