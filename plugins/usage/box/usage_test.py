"""Tests for usage.py's reading of Claude Code's `/usage`.

  python3 -m unittest plugins/usage/box/usage_test.py
"""
import datetime as dt
import importlib.util
import os
import pathlib
import unittest

spec = importlib.util.spec_from_file_location("usage", pathlib.Path(__file__).with_name("usage.py"))
usage = importlib.util.module_from_spec(spec)
spec.loader.exec_module(usage)

# What Claude Code 2.1.296 printed, word for word.
SAMPLE = """You are currently using your subscription to power your Claude Code usage

Current session: 31% used · resets Oct 15 at 3:30am (Asia/Saigon)
Current week (all models): 64% used · resets Oct 19 at 8am (Asia/Saigon)
Current week (Fable): 32% used · resets Oct 19 at 8am (Asia/Saigon)

What's contributing to your limits usage?
Last 24h · 318 requests · 5 sessions
  100% of your usage was at >150k context
"""

NOW = dt.datetime(2026, 10, 14, 12, 0, tzinfo=dt.timezone.utc)


def has_tzdata():
    try:
        from zoneinfo import ZoneInfo
        ZoneInfo("Asia/Saigon")
        return True
    except Exception:
        return False


# A box without the time zone database (a slim container) reads a printed
# zone as its own; the exact instants below need the database.
needs_tzdata = unittest.skipUnless(has_tzdata(), "no time zone database here")


def at(y, mo, d, h, mi, zone="Asia/Saigon"):
    from zoneinfo import ZoneInfo
    return int(dt.datetime(y, mo, d, h, mi, tzinfo=ZoneInfo(zone)).timestamp())


class ClaudeUsageTest(unittest.TestCase):
    @needs_tzdata
    def test_reads_the_session_the_week_and_a_models_week(self):
        l = usage.parse_claude_usage(SAMPLE, NOW)
        self.assertEqual(l["primary"], {"used_percent": 31.0, "window_minutes": 300, "resets_at": at(2026, 10, 15, 3, 30)})
        self.assertEqual(l["secondary"], {"used_percent": 64.0, "window_minutes": 10080, "resets_at": at(2026, 10, 19, 8, 0)})
        self.assertEqual(l["extra"], [{"used_percent": 32.0, "window_minutes": 10080, "resets_at": at(2026, 10, 19, 8, 0), "label": "Weekly limit (Fable)"}])

    def test_a_login_whose_usage_has_no_all_models_week(self):
        # Word for word what another Max login printed on 2026-10-15: a
        # session and a model's week, no week for all models.
        text = """You are currently using your subscription to power your Claude Code usage

Current session: 5% used · resets Oct 15 at 4:09pm (Asia/Saigon)
Current week (Fable): 0% used · resets Oct 21 at 3pm (Asia/Saigon)

What's contributing to your limits usage?
"""
        l = usage.parse_claude_usage(text, NOW)
        self.assertEqual(l["primary"]["used_percent"], 5.0)
        self.assertIsNone(l["secondary"])
        self.assertEqual([(w["label"], w["used_percent"]) for w in l["extra"]], [("Weekly limit (Fable)", 0.0)])

    def test_the_prose_below_is_no_limit(self):
        # "100% of your usage..." is about what drove the usage, not a window.
        l = usage.parse_claude_usage(SAMPLE, NOW)
        self.assertEqual(len(l["extra"]), 1)

    def test_no_session_line_means_no_reading(self):
        self.assertIsNone(usage.parse_claude_usage("Current week (all models): 4% used · resets Sep 14 at 5:59am (Asia/Jakarta)\n", NOW))
        self.assertIsNone(usage.parse_claude_usage("Usage is only available for subscription plans.\n", NOW))
        self.assertIsNone(usage.parse_claude_usage("", NOW))

    def test_a_reset_it_cannot_read_keeps_the_percentage(self):
        l = usage.parse_claude_usage("Current session: 38% used · resets in a little while\n", NOW)
        self.assertEqual(l["primary"]["used_percent"], 38.0)
        self.assertIsNone(l["primary"]["resets_at"])

    @needs_tzdata
    def test_resets_with_and_without_minutes(self):
        self.assertEqual(usage.reset_epoch("Sep 7 at 2:59pm (Asia/Jakarta)", dt.datetime(2026, 9, 7, tzinfo=dt.timezone.utc)), at(2026, 9, 7, 14, 59, "Asia/Jakarta"))
        self.assertEqual(usage.reset_epoch("Sep 7 at 3pm (Asia/Jakarta)", dt.datetime(2026, 9, 7, tzinfo=dt.timezone.utc)), at(2026, 9, 7, 15, 0, "Asia/Jakarta"))
        self.assertEqual(usage.reset_epoch("Sep 7 at 12am (Asia/Jakarta)", dt.datetime(2026, 9, 7, tzinfo=dt.timezone.utc)), at(2026, 9, 7, 0, 0, "Asia/Jakarta"))
        self.assertEqual(usage.reset_epoch("Sep 7 at 12pm (Asia/Jakarta)", dt.datetime(2026, 9, 7, tzinfo=dt.timezone.utc)), at(2026, 9, 7, 12, 0, "Asia/Jakarta"))

    @needs_tzdata
    def test_a_zone_with_am_in_its_name(self):
        self.assertEqual(usage.reset_epoch("Oct 19 at 8am (America/Panama)", NOW), at(2026, 10, 19, 8, 0, "America/Panama"))

    @needs_tzdata
    def test_the_year_nearest_now_across_new_year(self):
        eve = dt.datetime(2026, 12, 31, 12, tzinfo=dt.timezone.utc)
        self.assertEqual(usage.reset_epoch("Jan 2 at 9am (UTC)", eve), at(2027, 1, 2, 9, 0, "UTC"))
        new_year = dt.datetime(2027, 1, 2, 12, tzinfo=dt.timezone.utc)
        self.assertEqual(usage.reset_epoch("Dec 31 at 9am (UTC)", new_year), at(2026, 12, 31, 9, 0, "UTC"))

    def test_an_unknown_zone_is_read_as_the_boxs_own(self):
        # What a box without the zone database does with every zone.
        t = usage.reset_epoch("Oct 19 at 8am (Nowhere/Atlantis)", NOW)
        local = dt.datetime(2026, 10, 19, 8, 0).astimezone()
        self.assertEqual(t, int(local.timestamp()))

    def test_nonsense_is_none(self):
        self.assertIsNone(usage.reset_epoch("Smarch 3 at 9am (UTC)", NOW))
        self.assertIsNone(usage.reset_epoch("tomorrow", NOW))

    def test_a_setting_that_is_not_a_number_falls_back(self):
        for bad in ("0 BERTH_USAGE_CLAUDE_TIMEOUT=0.01", "ten", "", "-5"):
            os.environ["BERTH_USAGE_TEST_SECONDS"] = bad
            self.assertEqual(usage.env_seconds("BERTH_USAGE_TEST_SECONDS", "600"), 600.0, bad)
        os.environ["BERTH_USAGE_TEST_SECONDS"] = "0.5"
        self.assertEqual(usage.env_seconds("BERTH_USAGE_TEST_SECONDS", "600"), 0.5)
        del os.environ["BERTH_USAGE_TEST_SECONDS"]

    def test_the_script_starts_whatever_its_settings_say(self):
        import subprocess, sys
        env = {**os.environ, "BERTH_USAGE_CLAUDE_LIMITS_TTL": "0 oops", "BERTH_USAGE_CLAUDE_TIMEOUT": "x", "BERTH_USAGE_BUDGET": "y", "BERTH_USAGE_CLAUDE_LIMITS": "0"}
        r = subprocess.run([sys.executable, "-c", "import importlib.util,sys;s=importlib.util.spec_from_file_location('u',sys.argv[1]);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);print(m.CLAUDE_LIMITS_TTL,m.CLAUDE_USAGE_TIMEOUT,m.BUDGET_S)", str(pathlib.Path(usage.__file__))], env=env, capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout.split(), ["600.0", "12.0", "45.0"])

    def test_off_reads_nothing(self):
        os.environ["BERTH_USAGE_CLAUDE_LIMITS"] = "0"
        try:
            self.assertEqual(usage.claude_limits(), [])
        finally:
            del os.environ["BERTH_USAGE_CLAUDE_LIMITS"]


# A stand-in `claude`: what it prints, how long it takes and whether it
# fails are set per account through files in its config folder, and every
# run is logged with the CLAUDE_CONFIG_DIR it saw.
FAKE_CLAUDE = r"""#!/usr/bin/env python3
import os, subprocess, sys, time
home = os.environ.get("CLAUDE_CONFIG_DIR") or os.path.join(os.environ["FAKE_HOME"], ".claude")
with open(os.environ["FAKE_LOG"], "a") as f:
    f.write((os.environ.get("CLAUDE_CONFIG_DIR") or "-") + " " + " ".join(sys.argv[1:]) + " cwd=" + os.getcwd() + "\n")
def flag(n):
    return os.path.exists(os.path.join(home, n))
if flag("fail"):
    sys.exit(1)
if flag("apikey"):
    print("Usage is only available for subscription plans.")
    sys.exit(0)
pct = open(os.path.join(home, "pct")).read().strip() if flag("pct") else "10"
print("Current session: %s%% used · resets Oct 15 at 3:30am (Asia/Saigon)" % pct)
print("Current week (all models): 50% used · resets Oct 19 at 8am (Asia/Saigon)")
"""

HANG_WRAPPER = """#!/bin/sh
home="${CLAUDE_CONFIG_DIR:-$FAKE_HOME/.claude}"
if [ -e "$home/hang" ]; then
  sleep 60 &
  echo $! > "$home/child"
  sleep 60
fi
exec python3 "@FAKE@" "$@"
"""


class ClaudeLimitsTest(unittest.TestCase):
    def setUp(self):
        import tempfile, json
        self.tmp = tempfile.TemporaryDirectory()
        root = pathlib.Path(self.tmp.name)
        self.home = root / "home"
        (self.home / ".claude").mkdir(parents=True)
        (self.home / ".claude.json").write_text(json.dumps({"oauthAccount": {"emailAddress": "a@acme.test"}}))
        self.work = self.home / ".berth" / "accounts" / "claude" / "work"
        self.work.mkdir(parents=True)
        (self.work / ".claude.json").write_text(json.dumps({"oauthAccount": {"emailAddress": "b@acme.test"}}))
        (self.work / "pct").write_text("70")
        idle = self.home / ".berth" / "accounts" / "claude" / "signed-out"
        idle.mkdir(parents=True)
        bin_ = root / "bin"
        bin_.mkdir()
        (bin_ / "fake_claude.py").write_text(FAKE_CLAUDE)
        # The hanging case is shell, so it has started its child before any
        # timeout however busy the machine: a stuck Node helper of its own.
        (bin_ / "claude").write_text(HANG_WRAPPER.replace("@FAKE@", str(bin_ / "fake_claude.py")))
        (bin_ / "claude").chmod(0o755)
        self.log = root / "log"
        self.saved = {k: getattr(usage, k) for k in ("HOME", "ACCOUNTS", "CLAUDE_LIMITS_CACHE", "CLAUDE_LIMITS_TTL", "CLAUDE_USAGE_TIMEOUT", "CACHE")}
        usage.HOME, usage.ACCOUNTS = self.home, self.home / ".berth" / "accounts"
        usage.CLAUDE_LIMITS_CACHE = self.home / ".cache" / "berth-usage" / "claude-limits.json"
        usage.CACHE = self.home / ".cache" / "berth-usage" / "files.json"
        usage.CLAUDE_LIMITS_TTL, usage.CLAUDE_USAGE_TIMEOUT = 600, 5
        self.env = {k: os.environ.get(k) for k in ("PATH", "FAKE_HOME", "FAKE_LOG", "CLAUDE_CONFIG_DIR")}
        os.environ.update(PATH=str(bin_) + os.pathsep + os.environ["PATH"], FAKE_HOME=str(self.home), FAKE_LOG=str(self.log),
                          CLAUDE_CONFIG_DIR="/elsewhere/picked/by/the/worktree")

    def tearDown(self):
        for k, v in self.saved.items():
            setattr(usage, k, v)
        for k, v in self.env.items():
            if v is None:
                os.environ.pop(k, None)
            else:
                os.environ[k] = v
        self.tmp.cleanup()

    def runs(self):
        return self.log.read_text().splitlines() if self.log.exists() else []

    def by_account(self):
        return {l["account"]: l for l in usage.claude_limits()}

    def test_each_signed_in_login_is_read_with_its_own_folder(self):
        got = self.by_account()
        self.assertEqual(sorted(got), ["default", "work"])
        self.assertEqual(got["default"]["limits"]["primary"]["used_percent"], 10.0)
        self.assertEqual(got["work"]["limits"]["primary"]["used_percent"], 70.0)
        dirs = sorted(r.split(" ")[0] for r in self.runs())
        # default runs without CLAUDE_CONFIG_DIR, even when the worktree set one.
        self.assertEqual(dirs, ["-", str(self.work)])
        for r in self.runs():
            self.assertIn("--print --no-session-persistence --strict-mcp-config /usage", r)
            self.assertTrue(r.endswith(str(self.home / ".cache" / "berth-usage" / "claude-usage")), r)

    def test_a_read_is_kept_for_the_ttl(self):
        self.by_account()
        self.by_account()
        self.assertEqual(len(self.runs()), 2)
        usage.CLAUDE_LIMITS_TTL = 0
        self.by_account()
        self.assertEqual(len(self.runs()), 4)

    def test_a_failed_read_keeps_the_last_good_one(self):
        first = self.by_account()["work"]
        (self.work / "fail").touch()
        usage.CLAUDE_LIMITS_TTL = 0
        again = self.by_account()["work"]
        self.assertEqual(again["limits"], first["limits"])
        self.assertEqual(again["at"], first["at"])

    def test_a_login_without_limits_shows_none(self):
        (self.work / "apikey").touch()
        self.assertEqual(sorted(self.by_account()), ["default"])

    def test_hanging_logins_cost_one_timeout_and_leave_nothing_running(self):
        import time as t
        (self.home / ".claude" / "hang").touch()
        (self.work / "hang").touch()
        usage.CLAUDE_USAGE_TIMEOUT = 3
        started = t.monotonic()
        self.assertEqual(usage.claude_limits(), [])
        took = t.monotonic() - started
        # Both at once, each cut at its timeout: one timeout, not two.
        self.assertLess(took, 2 * usage.CLAUDE_USAGE_TIMEOUT, took)
        # And what Claude Code started goes with it, rather than left running.
        t.sleep(0.2)
        started = 0
        for d in (self.home / ".claude", self.work):
            # A read cut before the stand-in got going (a busy Mac vets a new
            # executable first) left nothing behind to check.
            if not (d / "child").exists():
                continue
            started += 1
            pid = int((d / "child").read_text())
            try:
                os.kill(pid, 0)
                alive = True
            except ProcessLookupError:
                alive = False
            self.assertFalse(alive, f"{d.name}'s child {pid} is still running")
        if not started:
            self.skipTest("neither stand-in started its child in time on this machine")

    def test_no_claude_is_no_limits(self):
        os.environ["PATH"] = "/nonexistent"
        saved = usage.claude_binary
        usage.claude_binary = lambda: None
        try:
            self.assertEqual(usage.claude_limits(), [])
        finally:
            usage.claude_binary = saved

    def test_the_report_survives_limits_that_break(self):
        import json
        saved = usage.claude_limits
        usage.claude_limits = lambda: (_ for _ in ()).throw(RuntimeError("boom"))
        try:
            r = json.loads(usage.report(7))
        finally:
            usage.claude_limits = saved
        self.assertEqual(r["limits"], [])
        self.assertIn("daily", r)

    def test_the_report_carries_claude_and_codex_defaults_apart(self):
        import json
        r = json.loads(usage.report(7))
        self.assertEqual(sorted((l["agent"], l["account"]) for l in r["limits"]), [("claude", "default"), ("claude", "work")])

    def test_the_report_says_whose_limits_they_are(self):
        import json
        r = json.loads(usage.report(7))
        self.assertEqual({l["account"]: l["email"] for l in r["limits"]}, {"default": "a@acme.test", "work": "b@acme.test"})


if __name__ == "__main__":
    unittest.main()
