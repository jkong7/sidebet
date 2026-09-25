import http.cookiejar
import json
import sys
import urllib.request

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8090"
EMAIL = sys.argv[2] if len(sys.argv) > 2 else "mod@u.northwestern.edu"
GROUP = "nu"

MARKETS = [
    ("Wildcats cover the spread this Saturday", 72),
    ("It snows in Evanston before Halloween", 24 * 35),
    ("Dillo Day headliner is announced before March 1", 24 * 150),
    ("The Lakefill freezes enough to walk on by February", 24 * 130),
    ("Norris Starbucks line is over 20 people at 10am Monday", 96),
    ("ASG's next referendum passes", 24 * 30),
    ("Northwestern makes a bowl game this season", 24 * 90),
    ("A Northwestern class gets canceled for snow this winter", 24 * 150),
]

opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))


def call(method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method, headers={"Content-Type": "application/json"})
    with opener.open(req) as r:
        return json.load(r)


start = call("POST", "/api/verify/start", {"email": EMAIL, "group": GROUP})
code = start.get("dev_code") or input(f"Code sent to {EMAIL}: ").strip()
call("POST", "/api/verify/finish", {"email": EMAIL, "code": code, "name": "sidebet mod"})
call("POST", f"/api/groups/{GROUP}/join")
for question, hours in MARKETS:
    m = call("POST", f"/api/groups/{GROUP}/markets", {"question": question, "closes_in_hours": hours})
    print(m["id"], question)
