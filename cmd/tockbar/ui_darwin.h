#ifndef TOCKBAR_UI_H
#define TOCKBAR_UI_H

void bar_run(void);
void bar_set_title(const char *title);
void bar_set_menu(const char *spec);
void bar_post(long long id, const char *title, const char *body, const char *category, const char *categories);
void bar_check_auth(int prompt);
void bar_open_url(const char *url);
void bar_quit(void);

#endif
