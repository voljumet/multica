import "../global.css";

import { useEffect, useRef } from "react";
import * as Notifications from "expo-notifications";
import { Stack, router } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { KeyboardProvider } from "react-native-keyboard-controller";
import { QueryClientProvider, useQueryClient } from "@tanstack/react-query";
import { ThemeProvider } from "@react-navigation/native";
import { PortalHost } from "@rn-primitives/portal";
import { api } from "@/data/api";
import { queryClient } from "@/data/query-client";
import { useAuthStore } from "@/data/auth-store";
import { useWorkspaceStore } from "@/data/workspace-store";
import { LightboxProvider, prewarmHighlighter } from "@/lib/markdown";
import { NAV_THEME } from "@/lib/theme";
import { useColorScheme } from "@/lib/use-color-scheme";

// Configure how push notifications are presented while the app is foregrounded.
// Must be at module level so it's set before any notification arrives.
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowAlert: true,
    shouldPlaySound: true,
    shouldSetBadge: false,
    shouldShowBanner: true,
    shouldShowList: true,
  }),
});

// Kick off Shiki highlighter init at module load — fires once per process,
// finishes before the user navigates to any screen with a code block. If
// init fails (engine unavailable) the highlighter falls back to plain
// text; nothing here is allowed to throw.
prewarmHighlighter();

function AuthInitializer({ children }: { children: React.ReactNode }) {
  const initialize = useAuthStore((s) => s.initialize);
  const qc = useQueryClient();
  // Idempotent guard: 401 on multiple in-flight requests would otherwise
  // logout/navigate repeatedly during the same session-expire moment.
  const signingOutRef = useRef(false);

  useEffect(() => {
    // Wire 401 handling onto the shared ApiClient singleton. Must be set
    // before any request fires — initialize() below kicks off the first
    // getMe() call, so do this synchronously first.
    api.setOptions({
      onUnauthorized: () => {
        if (signingOutRef.current) return;
        signingOutRef.current = true;
        void (async () => {
          await useAuthStore.getState().logout();
          await useWorkspaceStore.getState().clear();
          qc.clear();
          router.replace("/login");
          // Reset on next tick so a fresh session can hit 401 again later
          // without being silently swallowed.
          setTimeout(() => {
            signingOutRef.current = false;
          }, 0);
        })();
      },
    });
    initialize();
  }, [initialize, qc]);

  useEffect(() => {
    // Handle notification responses: long-press action or default tap.
    // Fires when the user taps a notification or activates an action button.
    const sub = Notifications.addNotificationResponseReceivedListener(
      (response) => {
        const data = response.notification.request.content.data as
          | { workspace_slug?: string; issue_id?: string }
          | undefined;

        // Long-press action: "Turn Off Notifications" — unsubscribe from the
        // issue and return. Do NOT navigate; the user dismissed the notification
        // without intending to open the app's issue view.
        if (response.actionIdentifier === "mute_issue") {
          if (data?.workspace_slug && data?.issue_id) {
            void api
              .unsubscribeFromIssue(data.issue_id, data.workspace_slug)
              .catch(() => {
                // Fire-and-forget: silently ignore network failures here.
                // The user will not receive notifications anyway because the
                // server's subscription check is the authority.
              });
          }
          return;
        }

        // Default tap: navigate to the issue.
        if (data?.workspace_slug && data?.issue_id) {
          router.push(
            `/${data.workspace_slug}/issue/${data.issue_id}` as Parameters<
              typeof router.push
            >[0]
          );
        }
      }
    );
    return () => sub.remove();
  }, []);

  return <>{children}</>;
}

export default function RootLayout() {
  const { colorScheme, isDarkColorScheme } = useColorScheme();
  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <SafeAreaProvider>
        <KeyboardProvider>
          <QueryClientProvider client={queryClient}>
            <ThemeProvider value={NAV_THEME[colorScheme]}>
              <AuthInitializer>
                <LightboxProvider>
                  <StatusBar style={isDarkColorScheme ? "light" : "dark"} />
                  <Stack screenOptions={{ headerShown: false }}>
                    <Stack.Screen name="index" />
                    <Stack.Screen name="(auth)" />
                    <Stack.Screen name="(app)" />
                  </Stack>
                  <PortalHost />
                </LightboxProvider>
              </AuthInitializer>
            </ThemeProvider>
          </QueryClientProvider>
        </KeyboardProvider>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}
