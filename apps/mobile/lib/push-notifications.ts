import * as Notifications from "expo-notifications";
import * as Device from "expo-device";
import Constants from "expo-constants";

/**
 * Requests iOS push permission and returns the Expo push token string,
 * or null when running on a simulator or permission is denied.
 *
 * Call once per workspace session (caller should persist the returned token
 * in expo-secure-store and skip re-registration when unchanged).
 */
export async function registerForPushNotifications(): Promise<string | null> {
  // Simulators cannot receive push notifications.
  if (!Device.isDevice) return null;

  const { status: existing } = await Notifications.getPermissionsAsync();
  let finalStatus = existing;

  if (existing !== "granted") {
    const { status } = await Notifications.requestPermissionsAsync();
    finalStatus = status;
  }

  if (finalStatus !== "granted") return null;

  // Register the iOS notification category that enables the long-press
  // "Turn Off Notifications" action. Safe to call on every registration —
  // Expo deduplicates it. Android ignores this silently.
  await Notifications.setNotificationCategoryAsync("issue_notification", [
    {
      identifier: "mute_issue",
      buttonTitle: "Turn Off Notifications",
      options: {
        isDestructive: false,
        isAuthenticationRequired: false,
      },
    },
  ]);

  const projectId =
    Constants.expoConfig?.extra?.eas?.projectId ??
    Constants.easConfig?.projectId;

  if (!projectId) {
    console.warn("[push] No EAS projectId found — push token unavailable");
    return null;
  }

  const { data: token } = await Notifications.getExpoPushTokenAsync({
    projectId,
  });
  return token;
}
