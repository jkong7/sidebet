import http.cookiejar
import json
import sys
import urllib.request

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8090"


class Friend:
    def __init__(self, name):
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        self.me = self.call("POST", "/api/me", {"name": name})

    def call(self, method, path, body=None):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(BASE + path, data=data, method=method, headers={"Content-Type": "application/json"})
        with self.opener.open(req) as r:
            return json.load(r)


marcus, priya, jake, sam = (Friend(n) for n in ("Marcus", "Priya", "Jake", "Sam"))
g = marcus.call("POST", "/api/groups", {"name": "apt 4b degenerates"})
code = g["code"]
for f in (priya, jake, sam):
    f.call("POST", f"/api/groups/{code}/join")
base = f"/api/groups/{code}"


def market(f, q, hours, subject=None):
    return f.call("POST", base + "/markets", {"question": q, "subject_id": subject and subject.me["id"], "closes_in_hours": hours})["id"]


def bet(f, m, side, amount):
    f.call("POST", f"{base}/markets/{m}/buy", {"side": side, "amount": amount})


m1 = market(marcus, "Does Jake text her back by Friday?", 72, jake)
m2 = market(jake, "Priya gets the Stripe offer", 168, priya)
m3 = market(priya, "Marcus orders DoorDash 5+ times this week", 120, marcus)
m4 = market(sam, "Jake actually goes to the gym tomorrow morning", 20, jake)
bet(priya, m1, "no", 180)
bet(jake, m1, "yes", 60)
bet(sam, m1, "no", 90)
bet(marcus, m2, "yes", 150)
bet(sam, m2, "yes", 80)
bet(jake, m3, "yes", 300)
bet(priya, m3, "yes", 120)
bet(marcus, m3, "no", 40)
bet(priya, m4, "no", 200)
print(json.dumps({"code": code, "markets": [m1, m2, m3, m4]}))
