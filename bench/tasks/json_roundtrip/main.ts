interface Rec {
  id: number;
  name: string;
  tags: string[];
  score: number;
  active: boolean;
}

let x = 7;
const recs: Rec[] = [];
for (let i = 0; i < 20_000; i++) {
  x = (x * 48271) % 2147483647;
  recs.push({ id: i, name: `user${i}`, tags: [`t${i % 7}`, `g${i % 11}`], score: x % 1000, active: x % 3 === 0 });
}
const text = JSON.stringify(recs);
const back: Rec[] = JSON.parse(text);
const score = back.reduce((s, r) => s + r.score, 0);
const active = back.filter((r) => r.active).length;
const tags = back.reduce((s, r) => s + r.tags.length, 0);
console.log(`bytes = ${new TextEncoder().encode(text).length}`);
console.log(`records = ${back.length} score = ${score} active = ${active} tags = ${tags}`);
