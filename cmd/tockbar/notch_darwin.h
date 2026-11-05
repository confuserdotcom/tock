#ifndef TOCKBAR_NOTCH_H
#define TOCKBAR_NOTCH_H

// notch_setup makes the notch panel and reports it with goNotch.
void notch_setup(void);
// bar_island draws the island JSON of bar.Island; "" hides the panel.
void bar_island(const char *spec);
// bar_notch_options: sharing lets screen sharing and recordings see the
// panel, all draws a notch on every display, hover opens it on a rest.
void bar_notch_options(int sharing, int all, int hover);

#endif
