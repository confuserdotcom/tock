#!/bin/sh
# macOS on a CI runner: no Accessibility grant there, so trackd sees the
# frontmost app but no window titles. The test matches app names.
set -eu
open -a TextEdit
open -a Calculator
sleep 3
# A runner that sat a few minutes with no input is idle to trackd, which
# then records Idle, rightly. nudge posts a zero-distance mouse move after
# each switch, as the Windows script sends F15.
nudge=${RUNNER_TEMP:-${TMPDIR:-/tmp}}/tock-nudge
swiftc -O -o "$nudge" scripts/e2e/nudge.swift
export E2E_TITLES=TextEdit,Calculator
export E2E_FOCUS="open -a \"{title}\" && $nudge"
go test -tags e2e ./e2e -v -count=1
