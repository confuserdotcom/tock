package main

import (
	"errors"
	"fmt"
	"time"

	"tock/internal/app"
	"tock/internal/recommend"
	"tock/internal/report"
	"tock/internal/store"
)

// cmdRecommend shows the pending recommendation, or accepts or dismisses it.
func cmdRecommend(env Env, args []string) error {
	if len(args) > 1 {
		return usagef("recommend takes accept or dismiss")
	}
	reply := ""
	if len(args) == 1 {
		switch args[0] {
		case "accept":
			reply = recommend.ReplyAccept
		case "dismiss":
			reply = recommend.ReplyDismiss
		default:
			return usagef("recommend: %q, want accept or dismiss", args[0])
		}
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	now := env.Now()
	r, err := a.PendingRecommend(now)
	if err != nil {
		return err
	}
	if reply == "" {
		if r == nil {
			fmt.Fprintln(env.Stdout, "no pending recommendation")
			return nil
		}
		until := time.UnixMilli(r.AtMS + recommend.PendingMS).In(now.Location())
		fmt.Fprintf(env.Stdout, "Suggested: %s · open until %s\n", report.RecText(*r), until.Format("15:04"))
		return nil
	}
	if r == nil {
		return app.ErrNoRecommend
	}
	// The row read above is the one answered: its send time guards against a
	// row that took the same id meanwhile.
	res, err := a.AnswerRecommendAt(r.ID, r.AtMS, "", reply)
	if errors.Is(err, store.ErrSessionRunning) {
		return errors.New("a session is already running")
	}
	if err != nil {
		return err
	}
	if res.BreakUntilMS > 0 {
		fmt.Fprintf(env.Stdout, "break until %s\n", res.BreakUntil.Format("15:04"))
		return nil
	}
	if res.Session != nil {
		fmt.Fprintf(env.Stdout, "started %d min session\n", (res.Session.PlannedMS-res.Session.StartMS)/60_000)
		return nil
	}
	fmt.Fprintf(env.Stdout, "dismissed: %s\n", report.RecText(res.Rec))
	return nil
}
