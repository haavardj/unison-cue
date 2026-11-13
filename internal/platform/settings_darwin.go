// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

@interface UCSettingsDelegate : NSObject <NSApplicationDelegate, NSWindowDelegate, WKNavigationDelegate>
@property (strong) NSWindow *window;
@property (strong) WKWebView *webView;
@property (copy) NSString *origin;
@end

@implementation UCSettingsDelegate
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)app { return YES; }

// Only the settings server may be shown in the window; anything else
// (there should be nothing) opens in the default browser.
- (void)webView:(WKWebView *)wv decidePolicyForNavigationAction:(WKNavigationAction *)action
    decisionHandler:(void (^)(WKNavigationActionPolicy))decision {
	NSURL *u = action.request.URL;
	NSString *o = [NSString stringWithFormat:@"%@://%@:%@", u.scheme, u.host, u.port];
	if ([o isEqualToString:self.origin]) { decision(WKNavigationActionPolicyAllow); return; }
	if ([u.scheme isEqualToString:@"http"] || [u.scheme isEqualToString:@"https"]) {
		[[NSWorkspace sharedWorkspace] openURL:u];
	}
	decision(WKNavigationActionPolicyCancel);
}
@end

static NSMenuItem *menuItem(NSString *title, SEL action, NSString *key) {
	return [[NSMenuItem alloc] initWithTitle:title action:action keyEquivalent:key];
}

// The main menu gives the window the standard shortcuts: Cmd-W, Cmd-Q,
// and copy/paste in text fields, which do not work without an Edit menu.
static void buildMenu(void) {
	NSMenu *bar = [NSMenu new];
	NSMenuItem *appItem = [NSMenuItem new];
	NSMenu *appMenu = [NSMenu new];
	[appMenu addItem:menuItem(@"Close Settings", @selector(terminate:), @"q")];
	appItem.submenu = appMenu;
	[bar addItem:appItem];

	NSMenuItem *editItem = [NSMenuItem new];
	NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
	[edit addItem:menuItem(@"Undo", @selector(undo:), @"z")];
	NSMenuItem *redo = menuItem(@"Redo", @selector(redo:), @"z");
	redo.keyEquivalentModifierMask = NSEventModifierFlagCommand | NSEventModifierFlagShift;
	[edit addItem:redo];
	[edit addItem:[NSMenuItem separatorItem]];
	[edit addItem:menuItem(@"Cut", @selector(cut:), @"x")];
	[edit addItem:menuItem(@"Copy", @selector(copy:), @"c")];
	[edit addItem:menuItem(@"Paste", @selector(paste:), @"v")];
	[edit addItem:menuItem(@"Select All", @selector(selectAll:), @"a")];
	editItem.submenu = edit;
	[bar addItem:editItem];

	NSMenuItem *winItem = [NSMenuItem new];
	NSMenu *win = [[NSMenu alloc] initWithTitle:@"Window"];
	[win addItem:menuItem(@"Close", @selector(performClose:), @"w")];
	[win addItem:menuItem(@"Minimize", @selector(performMiniaturize:), @"m")];
	winItem.submenu = win;
	[bar addItem:winItem];
	NSApp.mainMenu = bar;
}

static void runSettingsWindow(const char *curl) {
	@autoreleasepool {
		NSString *urlString = [NSString stringWithUTF8String:curl];
		NSURL *url = [NSURL URLWithString:urlString];
		[NSApplication sharedApplication];
		// A regular app while the window is open: Dock icon, Cmd-Tab, menu.
		[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
		buildMenu();

		UCSettingsDelegate *d = [UCSettingsDelegate new];
		d.origin = [NSString stringWithFormat:@"%@://%@:%@", url.scheme, url.host, url.port];
		NSApp.delegate = d;

		NSWindow *w = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 760, 600)
			styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
			          NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable
			backing:NSBackingStoreBuffered defer:NO];
		w.title = @"Unison Cue Settings";
		w.minSize = NSMakeSize(560, 420);
		w.releasedWhenClosed = NO;
		w.delegate = d;
		// Remember size and position between openings.
		[w setFrameAutosaveName:@"UnisonCueSettings"];
		if (![w setFrameUsingName:@"UnisonCueSettings"]) [w center];

		WKWebViewConfiguration *cfg = [WKWebViewConfiguration new];
		// A private, in-memory data store: nothing from the page is kept on disk.
		cfg.websiteDataStore = [WKWebsiteDataStore nonPersistentDataStore];
		WKWebView *wv = [[WKWebView alloc] initWithFrame:w.contentView.bounds configuration:cfg];
		wv.navigationDelegate = d;
		wv.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
		[wv setValue:@NO forKey:@"drawsBackground"]; // no white flash in dark mode
		w.contentView = wv;
		d.window = w;
		d.webView = wv;
		[wv loadRequest:[NSURLRequest requestWithURL:url]];

		[w makeKeyAndOrderFront:nil];
		[NSApp activateIgnoringOtherApps:YES];
		[NSApp run];
	}
}
*/
import "C"

import (
	"os/exec"
	"unsafe"
)

// SettingsWindowUnavailable is the exit code of the settings window
// process when no embedded browser is available. Never on macOS.
const SettingsWindowUnavailable = 3

// RunSettingsWindow shows url in a native window and returns when the
// window is closed. It must run on the main thread (systray's init locks
// it). dataDir is unused on macOS: the web view keeps no data.
func RunSettingsWindow(url, dataDir string) int {
	cs := C.CString(url)
	defer C.free(unsafe.Pointer(cs))
	C.runSettingsWindow(cs)
	return 0
}

// Reveal shows a file selected in the Finder.
func Reveal(path string) error { return exec.Command(openPath, "-R", path).Start() }
