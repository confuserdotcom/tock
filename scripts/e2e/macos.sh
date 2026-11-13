#!/bin/sh
# macOS on a CI runner: no Accessibility grant there, so trackd sees the
# frontmost app but no window titles. The test matches app names.
set -eu
open -a TextEdit
open -a Calculator
sleep 3
export E2E_TITLES=TextEdit,Calculator
export E2E_FOCUS='open -a "{title}"'
go test -tags e2e ./e2e -v -count=1
