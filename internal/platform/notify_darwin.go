// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

/*
#cgo LDFLAGS: -framework UserNotifications
#include <stdlib.h>
// Native notifications. Posting them from the app itself (rather than
// through osascript, whose notifications belong to Script Editor) puts
// our name and icon on them and lets a click open the sync log.

#import <Cocoa/Cocoa.h>
#import <UserNotifications/UserNotifications.h>

// Implemented in notify_fallback_darwin.go.
extern void ucNotifyFallback(char *title, char *body, char *reason);

static void ucOpen(NSString *target) {
	BOOL isDir = NO;
	if (![[NSFileManager defaultManager] fileExistsAtPath:target isDirectory:&isDir]) return;
	NSTask *task = [[NSTask alloc] init];
	task.executableURL = [NSURL fileURLWithPath:@"/usr/bin/open"];
	// Folders open in Finder; logs open in the default text editor.
	task.arguments = isDir ? @[@"--", target] : @[@"-t", @"--", target];
	[task launchAndReturnError:nil];
}

@interface UCNotifyDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation UCNotifyDelegate
// Show the banner even while the app counts as frontmost.
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))done {
	done(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList);
}

- (void)userNotificationCenter:(UNUserNotificationCenter *)center
didReceiveNotificationResponse:(UNNotificationResponse *)response
         withCompletionHandler:(void (^)(void))done {
	if ([response.actionIdentifier isEqualToString:UNNotificationDefaultActionIdentifier]) {
		id target = response.notification.request.content.userInfo[@"open"];
		if ([target isKindOfClass:[NSString class]] && [target length] > 0) ucOpen(target);
	}
	done();
}
@end

static UCNotifyDelegate *ucDelegate;

// UNUserNotificationCenter raises an exception outside an app bundle.
int ucNotifyAvailable(void) {
	return [[NSBundle mainBundle] bundleIdentifier] != nil;
}

void ucNotifyInit(void) {
	if (!ucNotifyAvailable() || ucDelegate) return;
	ucDelegate = [[UCNotifyDelegate alloc] init];
	[UNUserNotificationCenter currentNotificationCenter].delegate = ucDelegate;
}

void ucNotifyPost(const char *ctitle, const char *cbody, const char *ctarget) {
	NSString *title = [NSString stringWithUTF8String:ctitle];
	NSString *body = [NSString stringWithUTF8String:cbody];
	NSString *target = [NSString stringWithUTF8String:ctarget];
	void (^fallback)(NSError *) = ^(NSError *err) {
		ucNotifyFallback((char *)title.UTF8String, (char *)body.UTF8String,
			(char *)err.localizedDescription.UTF8String);
	};
	UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
	// Asks the user the first time; afterwards it answers at once.
	[center requestAuthorizationWithOptions:UNAuthorizationOptionAlert
	                      completionHandler:^(BOOL granted, NSError *err) {
		if (err) { fallback(err); return; }
		if (!granted) return; // the user turned our notifications off
		UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
		content.title = title;
		content.body = body;
		if (target.length > 0) content.userInfo = @{@"open": target};
		UNNotificationRequest *req =
			[UNNotificationRequest requestWithIdentifier:[[NSUUID UUID] UUIDString]
			                                     content:content trigger:nil];
		[center addNotificationRequest:req withCompletionHandler:^(NSError *err) {
			if (err) fallback(err);
		}];
	}];
}
*/
import "C"

import "unsafe"

// InitNotifications registers the handler for clicks on notifications.
// Call it at start-up, so a click that launches the app is handled too.
func InitNotifications() { C.ucNotifyInit() }

// Notify shows a notification through Notification Center. Clicking it
// opens target, a log file or a folder; target may be empty.
//
// Run from the app bundle, the notification is our own. Otherwise
// (go run, tests) it goes through osascript, which shows it as coming
// from Script Editor and cannot react to clicks.
func Notify(title, body, target string) error {
	if C.ucNotifyAvailable() == 0 {
		return notifyScript(title, body)
	}
	C.ucNotifyInit()
	ct, cb, cg := C.CString(title), C.CString(body), C.CString(target)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cb))
	defer C.free(unsafe.Pointer(cg))
	C.ucNotifyPost(ct, cb, cg)
	return nil
}

func notifyScript(title, body string) error {
	return runScript(`display notification (item 2 of argv) with title (item 1 of argv)`, title, body).Run()
}
