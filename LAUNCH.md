# Launch plan

Based on the 2026-09-25 Reels feed research (`~/dev/reels-research`). What performed:

| Signal from the feed | What it means for sidebet |
| --- | --- |
| Fantasy-football punishment (ACT retake) hit 8.5M plays | Friend-group stakes + public consequences is a proven format. sidebet is that, every week. |
| College life / nostalgia averaged ~3.8M | Film in dorms, apartments, dining halls. Real friend groups, not ads. |
| Dating and looks bait averaged ~1.3M, and the replies were the product | "Does he text her back" markets start the same comment fights, but about someone you know. |
| "Offer email hits in class" template reposted by multiple creators | "Priya gets the Stripe offer" markets ride the CS job-market genre. |
| Hooks: bold one-line overlay on a selfie-cam face, POV / "when that…" templates, confessional numbers | Every video opens on a face plus one line of text plus a big percentage. |

## Videos to shoot

1. **"My friends put odds on my love life."** Selfie cam, overlay: *my friends think there's an 18% chance he texts back*. Cut to the phone showing 18% and the chart dropping. End on the text arriving and the market settling YES while the group screams.
2. **"POV: you find out your roommate bet against you."** Show the trade feed: *Marcus bet 300 on NO*. Deadpan stare.
3. **Insider trading.** *He bet on his own market 🚨*. The flagged trade, the group chat roasting him, the odds swinging.
4. **The ALL IN.** Someone hits ALL IN on a 12% long shot. Freeze frame. Resolution. Either the loudest celebration of the week or a bailout-button walk of shame.
5. **Offer-drop crossover.** *The group had me at 26% to get the offer.* Open the email in class. Market settles YES. Leaderboard reshuffles.
6. **Leaderboard reveal.** Weekly: who's Top Degen, who's Down Bad. Loser does a punishment on camera. Series format, every Sunday.

## Mechanics that make it spread

- Every market link unfurls into a card with the odds, so the group chat does the distribution.
- Markets about a person pull that person in to defend themselves.
- The join screen shows the hottest market in the group before you sign up.
- No app store, no account: a link, a name, you're in.

## Guardrails that keep it launchable

- Play money only. Real-money betting on events is regulated gambling (state gaming laws; CFTC for event contracts), so cash never touches the app.
- The group owner can void any market. Reporting and "no markets about me" come before any paid acquisition.
- Target college and post-grad friend groups. Nothing in the marketing aimed at minors.

## Campus launch: Northwestern

Why a campus: Fizz reached 95% of Stanford undergrads by staying inside one verified school, and tbh set 40% of one school on day one as its bar. Density beats reach.

**Before launch**
- Deploy, set up SMTP, and add 3–5 student mods to `CAMPUS_ADMINS` (they settle markets, so pick people across different circles).
- Seed 8–12 markets with `scripts/seed_campus.py`: one sports market, one weather market, one Dillo Day market, one ASG market, one dining/Norris market.
- Pick one residential college or house as the beachhead.

**Launch day** (a home game Saturday or ASG election week)
- A table with free donuts outside Norris: verify on the spot, 1,000 coins, bet on the game.
- 3–5 paid ambassadors per beachhead dorm post the game market link in their group chats. Each link unfurls with live odds.
- Target: 40% of the beachhead dorm verified in 24 hours. Under 10% means fix the product before pushing harder.

**Every week**
- Sunday night: a mod posts the leaderboard's Top Degen and Down Bad as a story.
- Drop 2–3 new markets tied to what the campus is talking about that week.
- Settle markets quickly and publicly. Fast, fair settlement is the whole trust layer.
