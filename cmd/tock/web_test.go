package main

import "testing"

func TestWebUsage(t *testing.T) {
	te := newTestEnv(t, testRules)
	for _, args := range [][]string{{"web", "x"}, {"web", "--port", "-1"}, {"web", "--port", "70000"}, {"web", "--nope"}} {
		if code, _ := te.run(t, "", args...); code != 2 {
			t.Errorf("%v: exit %d", args, code)
		}
	}
}
