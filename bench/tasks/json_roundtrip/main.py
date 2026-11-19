import json

x = 7
recs = []
for i in range(20_000):
    x = x * 48271 % 2147483647
    recs.append({"id": i, "name": f"user{i}", "tags": [f"t{i % 7}", f"g{i % 11}"],
                 "score": x % 1000, "active": x % 3 == 0})
text = json.dumps(recs, separators=(",", ":"))
back = json.loads(text)
score = sum(r["score"] for r in back)
active = sum(1 for r in back if r["active"])
tags = sum(len(r["tags"]) for r in back)
print(f"bytes = {len(text.encode())}")
print(f"records = {len(back)} score = {score} active = {active} tags = {tags}")
