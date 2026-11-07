#import <Foundation/Foundation.h>
#import <WebKit/WebKit.h>
#import <objc/runtime.h>

// Wails' WailsContext is the webview's WKUIDelegate but does not implement the media capture
// permission method, so WebKit falls back to its own prompt on every launch. This adds it.
static void deputyRequestMediaCapture(id self, SEL _cmd, WKWebView *webView, WKSecurityOrigin *origin,
                                      WKFrameInfo *frame, WKMediaCaptureType type,
                                      void (^decisionHandler)(WKPermissionDecision)) {
    NSString *scheme = origin.protocol.lowercaseString;
    NSString *host = origin.host.lowercaseString;
    BOOL own = [scheme isEqualToString:@"wails"] ||
               ([scheme isEqualToString:@"http"] && ([host isEqualToString:@"localhost"] || [host isEqualToString:@"127.0.0.1"]));
    if (own && type == WKMediaCaptureTypeMicrophone) {
        decisionHandler(WKPermissionDecisionGrant);
    } else {
        decisionHandler(WKPermissionDecisionDeny);
    }
}

void DeputyAllowMediaCapture(void) {
    if (@available(macOS 12.0, *)) {
        Class cls = objc_getClass("WailsContext");
        if (cls == nil) return;
        SEL sel = @selector(webView:requestMediaCapturePermissionForOrigin:initiatedByFrame:type:decisionHandler:);
        if (class_getInstanceMethod(cls, sel) != NULL) return;
        class_addMethod(cls, sel, (IMP)deputyRequestMediaCapture, "v@:@@@q@?");
    }
}
