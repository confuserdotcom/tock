// dash_darwin.h: the dashboard window, a WKWebView on tock web's page.
#ifndef DASH_DARWIN_H
#define DASH_DARWIN_H

// bar_open_dashboard shows link in tock's dashboard window, creating it the
// first time. A link to the same server only changes the view.
void bar_open_dashboard(const char *link);

#endif
