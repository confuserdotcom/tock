// save_darwin.c: see save_darwin.h.
#include "save_darwin.h"

#include <copyfile.h>
#include <errno.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

int tock_is_local(const char *host) {
    return host && (!strcmp(host, "127.0.0.1") || !strcmp(host, "localhost") || !strcmp(host, "::1"));
}

int tock_nav_policy(const char *host, int download) {
    if (host && *host && !tock_is_local(host)) return TOCK_NAV_BROWSER;
    return download ? TOCK_NAV_DOWNLOAD : TOCK_NAV_ALLOW;
}

void tock_save_name(const char *suggested, char *out, size_t n) {
    if (!n) return;
    const char *s = suggested ? suggested : "";
    size_t end = strlen(s);
    while (end && s[end - 1] == '/') end--;
    size_t start = end;
    while (start && s[start - 1] != '/') start--;
    size_t len = end - start;
    if (!len || (len == 1 && s[start] == '.') || (len == 2 && s[start] == '.' && s[start + 1] == '.')) {
        snprintf(out, n, "tock-export");
        return;
    }
    snprintf(out, n, "%.*s", (int)len, s + start);
}

int tock_finish_save(const char *tmp, const char *dest) {
    if (rename(tmp, dest) == 0) return 0;
    if (errno != EXDEV) return errno;
    // Another volume: copy next to dest first, then rename there, so dest
    // still flips in one step.
    char part[4096];
    if (snprintf(part, sizeof part, "%s.tock-partial", dest) >= (int)sizeof part) return ENAMETOOLONG;
    if (copyfile(tmp, part, NULL, COPYFILE_DATA) != 0) {
        int e = errno;
        unlink(part);
        return e;
    }
    if (rename(part, dest) != 0) {
        int e = errno;
        unlink(part);
        return e;
    }
    unlink(tmp);
    return 0;
}
