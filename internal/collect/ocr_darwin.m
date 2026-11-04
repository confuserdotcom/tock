// On-device screen reading for "trackd ocr-worker" (OCR-01, OCR-03). It runs
// only in that short-lived child: one ScreenCaptureKit still of the front
// window, Vision text recognition in memory, the text on stdout. Nothing is
// written to disk and the image is released before the function returns.

#import <Cocoa/Cocoa.h>
#import <Carbon/Carbon.h>
#import <CoreText/CoreText.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <Vision/Vision.h>
#include <stdlib.h>
#include <string.h>
#include "ocr_darwin.h"

enum { OK = 0, NOPERM = 10, SECURE = 11, CHANGED = 12, NOWINDOW = 13, UNAVAILABLE = 14, ERR = 15 };

// The longer side of the image Vision reads, in pixels. Never below 1x:
// smaller text reads as nothing in fast mode.
static const double maxSide = 2200;

int tock_secure_input(void) { return IsSecureEventInputEnabled() ? 1 : 0; }
int tock_screen_preflight(void) { return CGPreflightScreenCaptureAccess() ? 1 : 0; }
int tock_screen_request(void) { return CGRequestScreenCaptureAccess() ? 1 : 0; }

// recognise runs Vision (fast, en + fr, no language correction) on img and
// returns the lines, one per observation.
static int recognise(CGImageRef img, char **out) {
	VNRecognizeTextRequest *r = [[VNRecognizeTextRequest alloc] init];
	r.recognitionLevel = VNRequestTextRecognitionLevelFast;
	r.usesLanguageCorrection = NO;
	r.recognitionLanguages = @[ @"en-US", @"fr-FR" ];
	VNImageRequestHandler *h = [[VNImageRequestHandler alloc] initWithCGImage:img options:@{}];
	NSError *err = nil;
	if (![h performRequests:@[ r ] error:&err]) return ERR;
	NSMutableString *text = [NSMutableString string];
	for (VNRecognizedTextObservation *o in r.results) {
		VNRecognizedText *c = [[o topCandidates:1] firstObject];
		if (c) [text appendFormat:@"%@\n", c.string];
	}
	*out = strdup(text.UTF8String);
	return *out ? OK : ERR;
}

int tock_ocr_run(const char *bundle, char **out) {
	@autoreleasepool {
		*out = NULL;
		if (!CGPreflightScreenCaptureAccess()) return NOPERM;
		if (IsSecureEventInputEnabled()) return SECURE;
		if (@available(macOS 14.0, *)) {
		} else {
			return UNAVAILABLE;
		}
		NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
		if (app == nil || bundle == NULL || ![app.bundleIdentifier isEqualToString:@(bundle)]) return CHANGED;

		// The front window of the app: the window list runs front to back.
		// Browsers keep untitled helper windows on layer 0 in front of the
		// real one (Arc: a score of 600x600), so the first titled window of
		// a real size wins, else the largest.
		CGWindowID wid = 0, largest = 0;
		double largestArea = 0;
		CFArrayRef arr = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
		if (arr) {
			for (NSDictionary *d in (__bridge NSArray *)arr) {
				if ([d[(id)kCGWindowOwnerPID] intValue] != app.processIdentifier || [d[(id)kCGWindowLayer] intValue] != 0 ||
				    [d[(id)kCGWindowAlpha] doubleValue] <= 0) {
					continue;
				}
				CGRect r;
				if (!CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)d[(id)kCGWindowBounds], &r)) continue;
				CGWindowID n = [d[(id)kCGWindowNumber] unsignedIntValue];
				NSString *name = d[(id)kCGWindowName];
				if (name.length > 0 && r.size.width >= 300 && r.size.height >= 200) {
					wid = n;
					break;
				}
				if (r.size.width * r.size.height > largestArea) {
					largestArea = r.size.width * r.size.height;
					largest = n;
				}
			}
			CFRelease(arr);
		}
		if (wid == 0) wid = largest;
		if (wid == 0) return NOWINDOW;

		__block SCShareableContent *content = nil;
		dispatch_semaphore_t s1 = dispatch_semaphore_create(0);
		[SCShareableContent getShareableContentExcludingDesktopWindows:YES
		                                           onScreenWindowsOnly:YES
		                                             completionHandler:^(SCShareableContent *c, NSError *e) {
			                                             content = c;
			                                             dispatch_semaphore_signal(s1);
		                                             }];
		if (dispatch_semaphore_wait(s1, dispatch_time(DISPATCH_TIME_NOW, 2 * NSEC_PER_SEC)) != 0) return ERR;
		if (content == nil) return NOPERM; // a lapsed approval shows up here
		SCWindow *w = nil;
		for (SCWindow *x in content.windows) {
			if (x.windowID == wid) {
				w = x;
				break;
			}
		}
		if (w == nil) return NOWINDOW;

		double width = w.frame.size.width, height = w.frame.size.height;
		if (width < 1 || height < 1) return NOWINDOW;
		double scale = MIN(1.0, maxSide / MAX(width, height));
		SCContentFilter *f = [[SCContentFilter alloc] initWithDesktopIndependentWindow:w];
		SCStreamConfiguration *cfg = [[SCStreamConfiguration alloc] init];
		cfg.width = (size_t)(width * scale);
		cfg.height = (size_t)(height * scale);
		cfg.showsCursor = NO;
		__block CGImageRef img = NULL;
		dispatch_semaphore_t s2 = dispatch_semaphore_create(0);
		[SCScreenshotManager captureImageWithFilter:f
		                              configuration:cfg
		                          completionHandler:^(CGImageRef i, NSError *e) {
			                          if (i) img = CGImageRetain(i);
			                          dispatch_semaphore_signal(s2);
		                          }];
		if (dispatch_semaphore_wait(s2, dispatch_time(DISPATCH_TIME_NOW, 2 * NSEC_PER_SEC)) != 0) return ERR;
		if (img == NULL) return ERR;
		int rc = recognise(img, out);
		CGImageRelease(img);
		return rc;
	}
}

// tock_ocr_render draws text (black Menlo 14 on white) and reads it back:
// a check of Vision that needs no permission.
int tock_ocr_render(const char *text, char **out) {
	@autoreleasepool {
		*out = NULL;
		size_t w = 1200, h = 200;
		CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
		CGContextRef ctx = CGBitmapContextCreate(NULL, w, h, 8, 0, cs, (CGBitmapInfo)kCGImageAlphaPremultipliedLast);
		CGColorSpaceRelease(cs);
		if (ctx == NULL) return ERR;
		CGContextSetRGBFillColor(ctx, 1, 1, 1, 1);
		CGContextFillRect(ctx, CGRectMake(0, 0, w, h));
		CTFontRef font = CTFontCreateWithName(CFSTR("Menlo"), 28, NULL);
		NSDictionary *attrs = @{(id)kCTFontAttributeName : (__bridge id)font, (id)kCTForegroundColorAttributeName : (__bridge id)CGColorGetConstantColor(kCGColorBlack)};
		NSAttributedString *s = [[NSAttributedString alloc] initWithString:@(text) attributes:attrs];
		CTLineRef line = CTLineCreateWithAttributedString((__bridge CFAttributedStringRef)s);
		CGContextSetTextPosition(ctx, 20, 90);
		CTLineDraw(line, ctx);
		CFRelease(line);
		CFRelease(font);
		CGImageRef img = CGBitmapContextCreateImage(ctx);
		CGContextRelease(ctx);
		if (img == NULL) return ERR;
		int rc = recognise(img, out);
		CGImageRelease(img);
		return rc;
	}
}
