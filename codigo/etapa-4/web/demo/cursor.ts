// The recorded video has no mouse pointer: this draws one (a dot that follows
// the mouse and a ring on each click) so the GIFs show where the action is.
// It only runs in the demo's browser, through addInitScript.
import type { BrowserContext } from "@playwright/test";

export async function showCursor(context: BrowserContext): Promise<void> {
  await context.addInitScript(() => {
    const install = () => {
      if (document.getElementById("demo-cursor") !== null) {
        return;
      }
      const dot = document.createElement("div");
      dot.id = "demo-cursor";
      Object.assign(dot.style, {
        position: "fixed", zIndex: "2147483647", pointerEvents: "none", width: "18px", height: "18px",
        marginLeft: "-9px", marginTop: "-9px", borderRadius: "50%", background: "rgba(220, 38, 38, 0.85)",
        border: "2px solid white", boxShadow: "0 0 4px rgba(0, 0, 0, 0.5)", left: "-50px", top: "-50px",
        transition: "left 0.25s ease-out, top 0.25s ease-out",
      });
      document.documentElement.appendChild(dot);
      document.addEventListener("mousemove", (e) => {
        dot.style.left = `${String(e.clientX)}px`;
        dot.style.top = `${String(e.clientY)}px`;
      }, true);
      document.addEventListener("mousedown", (e) => {
        const ring = document.createElement("div");
        Object.assign(ring.style, {
          position: "fixed", zIndex: "2147483647", pointerEvents: "none", width: "18px", height: "18px",
          marginLeft: "-9px", marginTop: "-9px", borderRadius: "50%", border: "3px solid rgb(220, 38, 38)",
          left: `${String(e.clientX)}px`, top: `${String(e.clientY)}px`, transition: "transform 0.4s, opacity 0.4s",
        });
        document.documentElement.appendChild(ring);
        requestAnimationFrame(() => {
          ring.style.transform = "scale(3)";
          ring.style.opacity = "0";
        });
        setTimeout(() => {
          ring.remove();
        }, 500);
      }, true);
    };
    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", install);
    } else {
      install();
    }
  });
}
