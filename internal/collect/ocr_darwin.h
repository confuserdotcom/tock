#ifndef TOCK_OCR_DARWIN_H
#define TOCK_OCR_DARWIN_H

// Exit-style codes shared with ocr.go: 0 ok, 10 no_permission, 11 secure,
// 12 changed, 13 no_window, 14 unavailable, 15 error.
int tock_ocr_run(const char *bundle, char **out);
int tock_ocr_render(const char *text, char **out);
int tock_secure_input(void);
int tock_screen_preflight(void);
int tock_screen_request(void);

#endif
