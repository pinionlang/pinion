let s = 0;
for (let i = 1; i <= 5_000_000; i++) {
  s += (i * i) % 1_000_003;
}
console.log(`sum = ${s}`);
