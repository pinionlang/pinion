def checksum(xs: list[int]) -> int:
    h = 0
    for v in xs:
        h = (h * 31 + v) % 1_000_000_007
    return h


x = 12345
nums = []
recs = []
for i in range(200_000):
    x = x * 48271 % 2147483647
    nums.append(x % 1_000_000)
    recs.append((x % 1000, i))
s = sorted(nums)
print(f"ints: min = {s[0]} max = {s[-1]} checksum = {checksum(s)}")
by_key = sorted(recs, key=lambda r: r[0])
print(f"recs: checksum = {checksum([r[1] for r in by_key])}")
