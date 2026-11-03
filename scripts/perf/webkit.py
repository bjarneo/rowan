"""Runs suite.js in an offscreen WebKitGTK view and prints the result as JSON.

    python3 webkit.py URL suite.js

No window opens. The view renders without the DMABUF renderer, because an
offscreen window has no GL context.
"""
import os
import sys

os.environ.setdefault("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
import gi

gi.require_version("Gtk", "3.0")
gi.require_version("WebKit2", "4.1")
from gi.repository import GLib, Gtk, WebKit2  # noqa: E402

url, path = sys.argv[1], sys.argv[2]
body = open(path, encoding="utf-8").read()
status = 1

win = Gtk.OffscreenWindow()
win.set_default_size(1440, 900)
view = WebKit2.WebView()
view.get_settings().set_enable_write_console_messages_to_stdout(True)
win.add(view)
win.show_all()


def finished(v, res, _data=None):
    global status
    try:
        print(v.call_async_javascript_function_finish(res).to_json(0))
        status = 0
    except GLib.Error as e:
        print("suite failed:", e.message, file=sys.stderr)
    Gtk.main_quit()


def changed(v, event):
    if event == WebKit2.LoadEvent.FINISHED:
        v.call_async_javascript_function(body, -1, None, None, None, None, finished, None)


view.connect("load-changed", changed)
view.load_uri(url)
GLib.timeout_add_seconds(600, Gtk.main_quit)
Gtk.main()
sys.exit(status)
