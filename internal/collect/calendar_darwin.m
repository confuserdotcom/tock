// EventKit access for trackd (CAL-01). It reads timed events only and never
// writes to a calendar.

#import <EventKit/EventKit.h>
#include <stdlib.h>
#include <string.h>
#include "calendar_darwin.h"

static EKEventStore *tockStore;

static EKEventStore *tock_store(void) {
	if (tockStore == nil) {
		tockStore = [[EKEventStore alloc] init];
	}
	return tockStore;
}

// tock_calendar_status returns EKAuthorizationStatus for events.
int tock_calendar_status(void) {
	return (int)[EKEventStore authorizationStatusForEntityType:EKEntityTypeEvent];
}

// tock_calendar_request asks for full access (macOS shows its prompt once)
// and returns the new status, or -1 when no answer came within two minutes.
int tock_calendar_request(void) {
	dispatch_semaphore_t sem = dispatch_semaphore_create(0);
	void (^done)(BOOL, NSError *) = ^(BOOL granted, NSError *err) {
		dispatch_semaphore_signal(sem);
	};
	if (@available(macOS 14.0, *)) {
		[tock_store() requestFullAccessToEventsWithCompletion:done];
	} else {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		[tock_store() requestAccessToEntityType:EKEntityTypeEvent completion:done];
#pragma clang diagnostic pop
	}
	if (dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 120 * NSEC_PER_SEC)) != 0) {
		return -1;
	}
	tockStore = nil; // a store made before the grant sees no calendars
	return tock_calendar_status();
}

static NSString *tock_clean(NSString *s) {
	if (s == nil) {
		return @"";
	}
	s = [s stringByReplacingOccurrencesOfString:@"\n" withString:@" "];
	return [s stringByReplacingOccurrencesOfString:@"\r" withString:@" "];
}

// tock_field cleans a middle field of an event line: newlines and tabs
// become spaces, and at most max characters are kept (composed character
// sequences are never cut in half).
static NSString *tock_field(NSString *s, NSUInteger max) {
	if (s == nil) {
		return @"";
	}
	if (s.length > max) {
		NSRange r = [s rangeOfComposedCharacterSequencesForRange:NSMakeRange(0, max)];
		if (r.length > max) {
			r = [s rangeOfComposedCharacterSequencesForRange:NSMakeRange(0, max - 1)];
		}
		s = [s substringWithRange:NSMakeRange(0, MIN(r.length, max))];
	}
	return [tock_clean(s) stringByReplacingOccurrencesOfString:@"\t" withString:@" "];
}

// tock_calendar_events returns the timed events overlapping [from_ms, to_ms)
// as "v2\tstart\tend\tcalendar\tlocation\turl\tnotes\ttitle\n" lines (malloc'd;
// the caller frees it). Notes are cut to their first 16384 characters.
// names holds calendar titles separated by "\n"; "" means every calendar.
// All-day, free and declined events are left out. NULL means failure.
char *tock_calendar_events(int64_t from_ms, int64_t to_ms, const char *names) {
	@autoreleasepool {
		EKEventStore *store = tock_store();
		NSArray<EKCalendar *> *cals = [store calendarsForEntityType:EKEntityTypeEvent];
		if (names != NULL && names[0] != '\0') {
			NSSet<NSString *> *want = [NSSet setWithArray:[[NSString stringWithUTF8String:names]
				componentsSeparatedByString:@"\n"]];
			NSMutableArray<EKCalendar *> *keep = [NSMutableArray array];
			for (EKCalendar *c in cals) {
				if ([want containsObject:c.title]) {
					[keep addObject:c];
				}
			}
			cals = keep;
		}
		if (cals.count == 0) {
			return strdup("");
		}
		NSDate *from = [NSDate dateWithTimeIntervalSince1970:(double)from_ms / 1000.0];
		NSDate *to = [NSDate dateWithTimeIntervalSince1970:(double)to_ms / 1000.0];
		NSPredicate *p = [store predicateForEventsWithStartDate:from endDate:to calendars:cals];
		NSArray<EKEvent *> *events = [store eventsMatchingPredicate:p];
		NSMutableString *out = [NSMutableString string];
		for (EKEvent *e in events) {
			if (e.allDay || e.availability == EKEventAvailabilityFree) {
				continue;
			}
			BOOL declined = NO;
			for (EKParticipant *a in e.attendees) {
				if (a.isCurrentUser && a.participantStatus == EKParticipantStatusDeclined) {
					declined = YES;
				}
			}
			if (declined) {
				continue;
			}
			long long start = llround(e.startDate.timeIntervalSince1970 * 1000.0);
			long long end = llround(e.endDate.timeIntervalSince1970 * 1000.0);
			[out appendFormat:@"v2\t%lld\t%lld\t%@\t%@\t%@\t%@\t%@\n", start, end,
				tock_field(e.calendar.title, NSUIntegerMax),
				tock_field(e.location, 4096),
				tock_field(e.URL.absoluteString, 2048),
				tock_field(e.notes, 16384),
				tock_clean(e.title)];
		}
		const char *utf8 = out.UTF8String;
		return utf8 == NULL ? NULL : strdup(utf8);
	}
}
