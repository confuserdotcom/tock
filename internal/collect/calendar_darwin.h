#include <stdint.h>

int tock_calendar_status(void);
int tock_calendar_request(void);
char *tock_calendar_events(int64_t from_ms, int64_t to_ms, const char *names);
