s = 0
for i in range(1, 5_000_001):
    s += i * i % 1_000_003
print(f"sum = {s}")
