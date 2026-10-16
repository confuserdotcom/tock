#ifndef TOCK_SHIM_H
#define TOCK_SHIM_H

typedef struct {
    char bundle[256];
    char app[256];
    char title[1024];
    int has_title;
    int ax_trusted;
    int ax_err;
    double idle_s;
    int locked;
    int video; // the front app or a helper of it keeps the display awake
} tock_poll_t;

void tock_ax_init(int prompt);
void tock_start_observers(void);
void tock_start_poll_timer(double seconds);
void tock_run_loop(void);
void tock_run_loop_slice(double seconds);
void tock_stop_run_loop(void);
void tock_poll(tock_poll_t *out);

#endif
