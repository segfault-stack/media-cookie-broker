import assert from "node:assert/strict";
import {readFile} from "node:fs/promises";
import test from "node:test";

import {runRecoveryAction} from "../popup-ui.js";

function resultHarness() {
  const ownerDocument = {
    createElement() {
      return {
        attributes: {},
        className: "",
        textContent: "",
        setAttribute(name, value) { this.attributes[name] = value; }
      };
    }
  };
  return {
    ownerDocument,
    children: [],
    replaceChildren(...children) { this.children = children; }
  };
}

test("failed recovery renders a wrapping inline popup error without a modal alert", async () => {
  let alertCalls = 0;
  globalThis.window = {alert() { alertCalls++; }};
  const button = {disabled: false, textContent: "Refresh session"};
  const resultElement = resultHarness();
  const message = "Chrome did not expose the cookie store for the recovery tab in time.";
  try {
    const response = await runRecoveryAction({
      button,
      resultElement,
      requestRecovery: async () => ({ok: false, error: message})
    });
    assert.deepEqual(response, {ok: false, error: message});
    assert.equal(alertCalls, 0);
    assert.equal(resultElement.children.length, 1);
    assert.equal(resultElement.children[0].className, "popup-error");
    assert.equal(resultElement.children[0].attributes.role, "alert");
    assert.equal(resultElement.children[0].textContent, message);
    assert.equal(button.disabled, false);
    assert.equal(button.textContent, "Refresh session");
  } finally {
    delete globalThis.window;
  }
});

test("popup control-plane code contains no modal alert or confirm calls", async () => {
  const source = await readFile(new URL("../popup.js", import.meta.url), "utf8");
  assert.doesNotMatch(source, /\b(?:window\.)?(?:alert|confirm)\s*\(/);
});
