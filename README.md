# sidebet

Put odds on your friends. Prediction markets for your group chat: make a yes/no bet about someone in the group, everyone trades play money on it, and the odds move live on every phone.

```
Does Jake text her back by Friday?          83%  ▁▂▁▃▂▇
Priya gets the Stripe offer                 89%
Marcus orders DoorDash 5+ times this week   74%
```

## How it works

- **Groups** are invite links. Anyone with the link picks a name and gets 1,000 coins.
- **Markets** are yes/no questions, optionally about a member. The creator settles them; the group owner can void any of them.
- **Odds** come from a logarithmic market scoring rule (LMSR) market maker, so every bet trades instantly and moves the price, with no order book and no counterparty needed. The house's worst-case loss per market is capped at `b·ln 2`.
- **Live**: every trade, new market and settlement is pushed to the group over Server-Sent Events.
- **Insider trading** is allowed and flagged. If the market is about you and you bet on it, everyone sees 🚨.
- **Share cards**: every group and market link renders a 1200×630 preview card (question, odds, sparkline), so dropping a link in iMessage or Discord shows the odds.
- **Broke?** One bailout of 100 coins a day.

Play money only. No deposits, no withdrawals, no cash prizes.

## Stack

Go standard library HTTP server, SQLite (pure Go, WAL), SSE, vanilla JS. One binary with the frontend embedded.

```
internal/lmsr   market maker math
internal/core   users, groups, markets, trades, payouts (SQLite)
internal/hub    per-group pub/sub for live events
internal/card   PNG share cards
internal/web    JSON API, SSE, pages with Open Graph tags, rate limiting
```

## Run

```sh
make run          # http://127.0.0.1:8090
make seed         # demo group with 4 friends and live markets
make test
```

## Deploy (Fly.io)

```sh
fly launch --no-deploy
fly volumes create sidebet_data --size 1
fly deploy
```

SQLite lives on the mounted volume. Cookies are `Secure` and client IPs come from `Fly-Client-IP` when `SECURE=1` and `TRUST_PROXY=1` (both set in the image).

## API

| Method | Path | |
| --- | --- | --- |
| GET/POST | `/api/me` | current user / pick or change your name |
| POST | `/api/groups` | create a group |
| GET | `/api/groups/{code}` | public preview: name, member count, hottest market |
| POST | `/api/groups/{code}/join` | join |
| GET/POST | `/api/groups/{code}/markets` | list / create |
| GET | `/api/groups/{code}/markets/{id}` | market, odds history, trades |
| POST | `/api/groups/{code}/markets/{id}/buy` | `{side, amount}` |
| POST | `/api/groups/{code}/markets/{id}/sell` | `{side, shares}` |
| POST | `/api/groups/{code}/markets/{id}/resolve` | `{outcome: yes, no or void}` |
| GET | `/api/groups/{code}/leaderboard` | net worth, marked to market |
| GET | `/api/groups/{code}/feed` | recent trades |
| POST | `/api/groups/{code}/bailout` | +100 once a day |
| GET | `/api/groups/{code}/events` | SSE stream |

## Roadmap

- [ ] Push notifications when someone bets on a market about you
- [ ] Weekly recap card: biggest win, worst beat, most insider trading
- [ ] Punishment markets: loser of the week does the dare (ACT-retake energy)
- [ ] iMessage app extension for betting without leaving the chat
- [ ] Reporting and an optional "no markets about me" setting per member
