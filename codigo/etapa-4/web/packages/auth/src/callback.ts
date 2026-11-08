// The script of /oidc-callback.html: answers the hidden frame of the session
// restore and the popup of the step-up, then the page is gone.
import { UserManager } from "oidc-client-ts";

export function completeCallback(): void {
  void new UserManager({ authority: "", client_id: "", redirect_uri: "" }).signinCallback();
}
