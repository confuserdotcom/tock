// save_darwin.h: the dashboard window's download decisions as plain C, so
// they can be tested from Go without a window or WebKit.
#ifndef TOCK_SAVE_DARWIN_H
#define TOCK_SAVE_DARWIN_H
#include <stddef.h>

enum { TOCK_NAV_ALLOW = 0, TOCK_NAV_BROWSER = 1, TOCK_NAV_DOWNLOAD = 2 };

// tock_nav_policy says what to do with a navigation: host is the URL's host
// (NULL when it has none, as for blob:), download whether the page asked
// for a download. Any other host goes to the browser first, download or not.
int tock_nav_policy(const char *host, int download);

// tock_is_local reports whether host is the loopback interface.
int tock_is_local(const char *host);

// tock_save_name writes the save sheet's default name for a suggested
// filename into out: its last path part, or "tock-export" when that is
// empty, "." or "..".
void tock_save_name(const char *suggested, char *out, size_t n);

// tock_finish_save moves the finished download tmp onto dest, replacing a
// file already there in one step. On failure dest is left as it was and
// the errno is returned; 0 on success. tmp is gone either way on success.
int tock_finish_save(const char *tmp, const char *dest);

#endif
