SYL = ["ka", "lo", "mi", "nu", "pe", "ra", "si", "to", "ve", "wa", "xi", "yo", "ze", "bu", "da", "fo"]

x = 42
parts = []
for _ in range(300_000):
    x = x * 48271 % 2147483647
    parts.append(SYL[x % 16] + SYL[x // 16 % 16])
text = " ".join(parts)

counts: dict[str, int] = {}
for w in text.split():
    counts[w] = counts.get(w, 0) + 1
ranked = sorted(counts.items(), key=lambda e: (-e[1], e[0]))
print(f"distinct = {len(ranked)}")
for w, c in ranked[:10]:
    print(f"{w} {c}")
