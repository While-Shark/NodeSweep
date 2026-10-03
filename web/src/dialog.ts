import { nextTick, type ObjectDirective } from "vue";
interface State {
  previous: HTMLElement | null;
  blocked: { element: HTMLElement; inert: boolean }[];
  keydown: (event: KeyboardEvent) => void;
  close: () => void;
}
const states = new WeakMap<HTMLElement, State>();
const focusable = (element: HTMLElement) =>
  Array.from(
    element.querySelectorAll<HTMLElement>(
      'button:not(:disabled),input:not(:disabled):not([type="hidden"]),select:not(:disabled),textarea:not(:disabled),a[href],[tabindex]:not([tabindex="-1"])',
    ),
  ).filter(
    (target) => target.getClientRects().length && !target.closest("[inert]"),
  );
function active(element: HTMLElement) {
  return (
    Array.from(document.querySelectorAll('[aria-modal="true"]')).at(-1) ===
    element
  );
}
export const dialog: ObjectDirective<HTMLElement, () => void> = {
  mounted(element, binding) {
    const state: State = {
      previous:
        document.activeElement instanceof HTMLElement
          ? document.activeElement
          : null,
      blocked: [],
      close: binding.value,
      keydown: () => {},
    };
    // Inert siblings at every ancestor level so the backdrop stays reachable
    // while controls behind the dialog leave the keyboard/accessibility tree.
    let ancestor: HTMLElement = element;
    while (ancestor.parentElement) {
      for (const sibling of Array.from(ancestor.parentElement.children)) {
        if (
          sibling !== ancestor &&
          sibling instanceof HTMLElement &&
          !["SCRIPT", "STYLE"].includes(sibling.tagName)
        ) {
          state.blocked.push({ element: sibling, inert: sibling.inert });
          sibling.inert = true;
        }
      }
      ancestor = ancestor.parentElement;
      if (ancestor === document.body) break;
    }
    state.keydown = (event) => {
      if (!active(element)) return;
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        state.close();
        return;
      }
      if (event.key !== "Tab") return;
      const controls = focusable(element);
      const first = controls[0],
        last = controls.at(-1);
      if (!first) {
        event.preventDefault();
        element.focus();
        return;
      }
      if (
        event.shiftKey &&
        (document.activeElement === first ||
          !element.contains(document.activeElement))
      ) {
        event.preventDefault();
        last!.focus();
      } else if (
        !event.shiftKey &&
        (document.activeElement === last ||
          !element.contains(document.activeElement))
      ) {
        event.preventDefault();
        first.focus();
      }
    };
    states.set(element, state);
    document.addEventListener("keydown", state.keydown, true);
    element.tabIndex = -1;
    void nextTick(() => {
      if (element.isConnected && active(element))
        (focusable(element)[0] || element).focus();
    });
  },
  updated(element, binding) {
    const state = states.get(element);
    if (state) state.close = binding.value;
    if (active(element) && !element.contains(document.activeElement))
      void nextTick(() => {
        if (element.isConnected && active(element))
          (focusable(element)[0] || element).focus();
      });
  },
  unmounted(element) {
    const state = states.get(element);
    if (!state) return;
    document.removeEventListener("keydown", state.keydown, true);
    for (const item of state.blocked) item.element.inert = item.inert;
    if (state.previous?.isConnected && !state.previous.closest("[inert]"))
      state.previous.focus();
    states.delete(element);
  },
};
