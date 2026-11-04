// tock: send the front tab to trackd (native messaging host dev.tock.browser).
// Incognito tabs send no address or title, only that they are incognito.
"use strict";
const HOST = "dev.tock.browser";
let port = null;

function send(msg) {
  if (!port) {
    try {
      port = chrome.runtime.connectNative(HOST);
      port.onDisconnect.addListener(() => { port = null; });
    } catch (e) {
      port = null;
      return;
    }
  }
  try { port.postMessage(msg); } catch (e) { port = null; }
}

async function report() {
  let win = null;
  try { win = await chrome.windows.getLastFocused(); } catch (e) { win = null; }
  if (!win || !win.focused) { send({ focused: false }); return; }
  let tab = null;
  try { [tab] = await chrome.tabs.query({ active: true, windowId: win.id }); } catch (e) { tab = null; }
  if (!tab) { send({ focused: false }); return; }
  if (tab.incognito) { send({ focused: true, incognito: true }); return; }
  send({ focused: true, url: tab.url || "", title: tab.title || "" });
}

chrome.tabs.onActivated.addListener(report);
chrome.tabs.onUpdated.addListener((id, change, tab) => {
  if (tab.active && (change.url !== undefined || change.title !== undefined)) report();
});
chrome.windows.onFocusChanged.addListener(report);
chrome.runtime.onStartup.addListener(report);
report();
