from __future__ import annotations

import asyncio
import contextlib
import re
import threading
from concurrent.futures import Future, TimeoutError
from dataclasses import dataclass
from typing import Any, Callable, Coroutine, Dict, List, Optional, Tuple

try:
    import curses
except ImportError as exc:  # pragma: no cover - platform dependent
    curses = None  # type: ignore[assignment]
    _CURSES_IMPORT_ERROR = exc
else:  # pragma: no cover - platform dependent
    _CURSES_IMPORT_ERROR = None

from . import discovery, wizlight
from .bulb import PilotBuilder
from .scenes import SCENES

RGBTuple = Tuple[int, int, int]


@dataclass
class BulbInfo:
    """Container for wizlight instances and their state."""

    device: wizlight
    ip: str
    mac: Optional[str]
    state: Optional[bool]
    brightness: Optional[int] = None
    rgb: Optional[RGBTuple] = None
    scene_id: Optional[int] = None
    last_error: Optional[str] = None

    @property
    def label(self) -> str:
        return self.mac or self.ip


class WizAsyncController:
    """Manage wizlight coroutines on a dedicated asyncio loop."""

    def __init__(self, broadcast_address: str, wait_time: float) -> None:
        self.broadcast_address = broadcast_address
        self.wait_time = wait_time
        self._loop = asyncio.new_event_loop()
        self._thread = threading.Thread(target=self._run_loop, daemon=True)
        self._lights: Dict[str, wizlight] = {}
        self._thread.start()

    def _run_loop(self) -> None:
        asyncio.set_event_loop(self._loop)
        self._loop.run_forever()

    def _submit(self, coro: Coroutine[Any, Any, Dict[str, Optional[Any]]]) -> Future:
        return asyncio.run_coroutine_threadsafe(coro, self._loop)

    def discover(self) -> List[BulbInfo]:
        future = self._submit(self._discover())
        timeout = max(5.0, self.wait_time + 2.0)
        return future.result(timeout=timeout)

    def set_power(self, bulb: BulbInfo, turn_on: bool) -> Dict[str, Optional[Any]]:
        future = self._submit(self._set_power(bulb.device, turn_on))
        return future.result(timeout=10.0)

    def set_scene(self, bulb: BulbInfo, scene_id: int) -> Dict[str, Optional[Any]]:
        future = self._submit(self._set_scene(bulb.device, scene_id))
        return future.result(timeout=10.0)

    def set_brightness(self, bulb: BulbInfo, brightness: int) -> Dict[str, Optional[Any]]:
        future = self._submit(self._set_brightness(bulb.device, brightness))
        return future.result(timeout=10.0)

    def set_rgb(self, bulb: BulbInfo, rgb: RGBTuple) -> Dict[str, Optional[Any]]:
        future = self._submit(self._set_rgb(bulb.device, rgb))
        return future.result(timeout=10.0)

    def refresh_state(self, bulb: BulbInfo) -> Dict[str, Optional[Any]]:
        future = self._submit(self._refresh_state(bulb.device))
        return future.result(timeout=10.0)

    def shutdown(self) -> None:
        if self._loop.is_closed():
            return
        close_future = self._submit(self._shutdown_lights())
        try:
            close_future.result(timeout=5.0)
        except TimeoutError:
            pass
        self._loop.call_soon_threadsafe(self._loop.stop)
        self._thread.join(timeout=5.0)
        if self._thread.is_alive():
            return
        self._loop.close()

    async def _discover(self) -> List[BulbInfo]:
        discovered = await discovery.find_wizlights(
            wait_time=self.wait_time, broadcast_address=self.broadcast_address
        )
        found_ips = {entry.ip_address: entry for entry in discovered}

        for ip in list(self._lights):
            if ip not in found_ips:
                light = self._lights.pop(ip)
                with contextlib.suppress(Exception):
                    await light.async_close()

        bulbs: List[BulbInfo] = []
        for ip, entry in found_ips.items():
            light = self._lights.get(ip)
            if light is None:
                light = wizlight(ip=entry.ip_address, mac=entry.mac_address)
                self._lights[ip] = light
            elif light.mac is None:
                light.mac = entry.mac_address

            state: Optional[bool] = None
            brightness: Optional[int] = None
            rgb: Optional[RGBTuple] = None
            scene_id: Optional[int] = None
            error: Optional[str] = None
            try:
                parser = await light.updateState()
            except Exception as exc:  # pragma: no cover - network dependent
                error = str(exc)
                parser = None
            else:
                details = self._extract_details(parser)
                state = details["state"]
                brightness = details["brightness"]
                rgb = details["rgb"]
                scene_id = details["scene_id"]
                if details["mac"] and light.mac is None:
                    light.mac = details["mac"]

            bulbs.append(
                BulbInfo(
                    device=light,
                    ip=ip,
                    mac=light.mac,
                    state=state,
                    brightness=brightness,
                    rgb=rgb,
                    scene_id=scene_id,
                    last_error=error,
                )
            )

        bulbs.sort(key=lambda info: info.ip)
        return bulbs

    async def _set_power(self, device: wizlight, turn_on: bool) -> Dict[str, Optional[Any]]:
        try:
            if turn_on:
                await device.turn_on()
            else:
                await device.turn_off()
            parser = await device.updateState()
        except Exception as exc:  # pragma: no cover - network dependent
            return {**self._empty_result(), "error": str(exc)}
        details = self._extract_details(parser)
        if parser is None:
            details["state"] = True if turn_on else False
        return {**details, "error": None}

    async def _set_scene(self, device: wizlight, scene_id: int) -> Dict[str, Optional[Any]]:
        builder = PilotBuilder(scene=scene_id, state=True)
        return await self._apply_builder(device, builder)

    async def _set_brightness(self, device: wizlight, brightness: int) -> Dict[str, Optional[Any]]:
        builder = PilotBuilder(brightness=brightness, state=True)
        return await self._apply_builder(device, builder)

    async def _set_rgb(self, device: wizlight, rgb: RGBTuple) -> Dict[str, Optional[Any]]:
        builder = PilotBuilder(rgb=rgb, state=True)
        return await self._apply_builder(device, builder)

    async def _apply_builder(
        self, device: wizlight, builder: PilotBuilder
    ) -> Dict[str, Optional[Any]]:
        try:
            await device.turn_on(builder)
            parser = await device.updateState()
        except Exception as exc:  # pragma: no cover - network dependent
            return {**self._empty_result(), "error": str(exc)}
        details = self._extract_details(parser)
        return {**details, "error": None}

    async def _refresh_state(self, device: wizlight) -> Dict[str, Optional[Any]]:
        try:
            parser = await device.updateState()
        except Exception as exc:  # pragma: no cover - network dependent
            return {**self._empty_result(), "error": str(exc)}
        details = self._extract_details(parser)
        return {**details, "error": None}

    async def _shutdown_lights(self) -> None:
        for light in list(self._lights.values()):
            with contextlib.suppress(Exception):
                await light.async_close()
        self._lights.clear()

    @staticmethod
    def _empty_result() -> Dict[str, Optional[Any]]:
        return {
            "state": None,
            "brightness": None,
            "rgb": None,
            "scene_id": None,
            "mac": None,
            "error": None,
        }

    @staticmethod
    def _extract_details(parser: Optional[Any]) -> Dict[str, Optional[Any]]:
        details = WizAsyncController._empty_result()
        if parser is None:
            return details
        state = parser.get_state()
        brightness = parser.get_brightness()
        rgb_value: Optional[RGBTuple] = None
        rgb_raw = parser.get_rgb()
        if rgb_raw:
            try:
                rgb_tuple = tuple(rgb_raw)
            except TypeError:
                rgb_tuple = None
            if rgb_tuple and len(rgb_tuple) >= 3 and None not in rgb_tuple[:3]:
                rgb_value = (
                    int(rgb_tuple[0]),
                    int(rgb_tuple[1]),
                    int(rgb_tuple[2]),
                )
        scene_id = parser.get_scene_id()
        mac = parser.get_mac()
        details.update(
            {
                "state": state,
                "brightness": brightness,
                "rgb": rgb_value,
                "scene_id": scene_id,
                "mac": mac,
            }
        )
        return details


class WizTUI:
    """Curses UI that lists bulbs and allows power control."""

    def __init__(self, stdscr: "curses._CursesWindow", controller: WizAsyncController) -> None:
        self.stdscr = stdscr
        self.controller = controller
        self.bulbs: List[BulbInfo] = []
        self.selected_index = 0
        self.status_message = "Press r to scan for bulbs"
        self.show_scene_list = False
        self.scene_list_index = 0
        self.group_mode = False
        self._init_default_attrs()

    def _init_default_attrs(self) -> None:
        dim = getattr(curses, "A_DIM", curses.A_NORMAL)
        self.attr_header = curses.A_REVERSE | curses.A_BOLD
        self.attr_footer = curses.A_BOLD
        self.attr_footer_info = curses.A_BOLD
        self.attr_footer_error = curses.A_BOLD | curses.A_REVERSE
        self.attr_row = curses.A_NORMAL
        self.attr_row_alt = dim
        self.attr_row_on = curses.A_BOLD
        self.attr_row_off = dim
        self.attr_row_selected = curses.A_REVERSE | curses.A_BOLD
        self.attr_scene_row = curses.A_NORMAL
        self.attr_scene_selected = curses.A_REVERSE | curses.A_BOLD
        self.attr_scene_hint = dim

    def _init_colors(self) -> None:
        if not curses.has_colors():
            return
        try:
            curses.start_color()
        except curses.error:
            return
        with contextlib.suppress(curses.error):
            curses.use_default_colors()

        def init_pair(idx: int, fg: int, bg: int = -1) -> int:
            try:
                curses.init_pair(idx, fg, bg)
                return curses.color_pair(idx)
            except curses.error:
                return 0

        header = init_pair(1, curses.COLOR_BLACK, curses.COLOR_CYAN)
        if header:
            self.attr_header = header | curses.A_BOLD

        footer = init_pair(2, curses.COLOR_WHITE, curses.COLOR_BLUE)
        if footer:
            self.attr_footer = footer | curses.A_BOLD

        info = init_pair(3, curses.COLOR_GREEN, -1)
        if info:
            self.attr_footer_info = info | curses.A_BOLD

        error = init_pair(4, curses.COLOR_WHITE, curses.COLOR_RED)
        if error:
            self.attr_footer_error = error | curses.A_BOLD

        row = init_pair(5, curses.COLOR_WHITE, -1)
        alt = init_pair(6, curses.COLOR_CYAN, -1)
        selected = init_pair(7, curses.COLOR_BLACK, curses.COLOR_YELLOW)
        dim = getattr(curses, "A_DIM", curses.A_NORMAL)
        if row:
            self.attr_row = row
        if alt:
            self.attr_row_alt = alt | dim
        if selected:
            self.attr_row_selected = selected | curses.A_BOLD
            self.attr_scene_selected = selected | curses.A_BOLD
            self.attr_row_on = curses.A_BOLD
            self.attr_row_off = dim

        scene_row = init_pair(8, curses.COLOR_CYAN, -1)
        if scene_row:
            self.attr_scene_row = scene_row | curses.A_BOLD
        hint = init_pair(9, curses.COLOR_MAGENTA, -1)
        if hint:
            self.attr_scene_hint = hint | dim

    def run(self) -> None:
        try:
            curses.curs_set(0)
        except curses.error:  # pragma: no cover - terminal dependent
            pass
        self.stdscr.nodelay(False)
        self.stdscr.keypad(True)
        self._init_colors()
        self.refresh_bulbs(initial=True)

        while True:
            self.draw()
            key = self.stdscr.getch()
            if key in (ord("q"), 27):
                break
            if self.show_scene_list and self._handle_scene_list_key(key):
                continue
            if key in (curses.KEY_UP, ord("k")):
                self._move_selection(-1)
            elif key in (curses.KEY_DOWN, ord("j")):
                self._move_selection(1)
            elif key in (ord("r"), ord("R")):
                self.refresh_bulbs()
            elif key in (ord(" "), ord("t"), ord("T")):
                self.toggle_selected()
            elif key in (ord("o"), ord("O")):
                self.set_selected(True)
            elif key in (ord("f"), ord("F")):
                self.set_selected(False)
            elif key in (ord("s"), ord("S")):
                self.refresh_selected()
            elif key in (ord("b"), ord("B")):
                self.adjust_brightness()
            elif key in (ord("c"), ord("C")):
                self.set_rgb_color()
            elif key in (ord("n"), ord("N")):
                self.apply_scene()
            elif key in (ord("g"), ord("G")):
                self.toggle_group_mode()
            elif key in (ord("l"), ord("L")):
                self.toggle_scene_list()

    def refresh_bulbs(self, initial: bool = False) -> None:
        self.status_message = "Scanning for bulbs..."
        self.draw()
        try:
            self.bulbs = self.controller.discover()
        except Exception as exc:  # pragma: no cover - network dependent
            self.bulbs = []
            self.selected_index = 0
            self.status_message = f"Discovery failed: {exc}"
            self.draw()
            return
        if self.selected_index >= len(self.bulbs):
            self.selected_index = max(len(self.bulbs) - 1, 0)
        if not self.bulbs:
            self.group_mode = False
            self.status_message = "No bulbs found. Press r to retry."
        else:
            count = len(self.bulbs)
            prefix = "Found" if not initial else "Discovered"
            base_msg = f"{prefix} {count} bulb{'s' if count != 1 else ''}."
            if self.show_scene_list:
                self.status_message = self._scene_list_status()
            else:
                self.status_message = base_msg + ' Press l to view scenes.'
        self.draw()

    def toggle_selected(self) -> None:
        targets = self._targets()
        if not targets:
            return
        known_states = [state for state in (bulb.state for bulb in targets) if state is not None]
        desired = False if known_states and all(known_states) else True
        verb = "on" if desired else "off"
        self.status_message = f"Turning {verb} {self._target_label(targets)}..."
        self.draw()
        success, failures = self._apply_to_targets(
            targets, lambda bulb: self.controller.set_power(bulb, desired)
        )
        if failures:
            self.status_message = self._format_failure_summary(
                f"Turned {verb}", success, len(targets), failures
            )
        else:
            if len(targets) == 1:
                bulb = targets[0]
                status = "on" if bulb.state else "off"
                self.status_message = f"{bulb.label} is {status}."
            else:
                self.status_message = f"Turned {verb} {success} bulbs."
        self.draw()

    def set_selected(self, turn_on: bool) -> None:
        targets = self._targets()
        if not targets:
            return
        verb = "on" if turn_on else "off"
        self.status_message = f"Turning {verb} {self._target_label(targets)}..."
        self.draw()
        success, failures = self._apply_to_targets(
            targets, lambda bulb: self.controller.set_power(bulb, turn_on)
        )
        if failures:
            self.status_message = self._format_failure_summary(
                f"Turned {verb}", success, len(targets), failures
            )
        else:
            if len(targets) == 1:
                bulb = targets[0]
                status = "on" if bulb.state else "off"
                self.status_message = f"{bulb.label} is {status}."
            else:
                self.status_message = f"Turned {verb} {success} bulbs."
        self.draw()

    def refresh_selected(self) -> None:
        targets = self._targets()
        if not targets:
            return
        self.status_message = f"Refreshing {self._target_label(targets)}..."
        self.draw()
        success, failures = self._apply_to_targets(
            targets, lambda bulb: self.controller.refresh_state(bulb)
        )
        if failures:
            self.status_message = self._format_failure_summary(
                "Refreshed", success, len(targets), failures
            )
        else:
            if len(targets) == 1:
                bulb = targets[0]
                self.status_message = f"{bulb.label}: {self._format_status_summary(bulb)}"
            else:
                self.status_message = f"Refreshed {success} bulbs."
        self.draw()

    def adjust_brightness(self) -> None:
        targets = self._targets()
        if not targets:
            return
        response = self._prompt("Brightness 0-255 (blank to cancel): ")
        if response is None:
            self.status_message = "Brightness update cancelled."
            self.draw()
            return
        try:
            value = int(response)
        except ValueError:
            self.status_message = "Brightness must be an integer between 0 and 255."
            self.draw()
            return
        if not 0 <= value <= 255:
            self.status_message = "Brightness must be between 0 and 255."
            self.draw()
            return
        self.status_message = f"Setting brightness {value} for {self._target_label(targets)}..."
        self.draw()
        success, failures = self._apply_to_targets(
            targets, lambda bulb: self.controller.set_brightness(bulb, value)
        )
        if failures:
            self.status_message = self._format_failure_summary(
                "Brightness set on", success, len(targets), failures
            )
        else:
            if len(targets) == 1:
                bulb = targets[0]
                self.status_message = f"{bulb.label} brightness {bulb.brightness}."
            else:
                self.status_message = f"Set brightness {value} on {success} bulbs."
        self.draw()

    def set_rgb_color(self) -> None:
        targets = self._targets()
        if not targets:
            return
        response = self._prompt("RGB (e.g. 255,128,0 or #FF8000): ")
        if response is None:
            self.status_message = "RGB update cancelled."
            self.draw()
            return
        try:
            rgb = self._parse_rgb_input(response)
        except ValueError as exc:
            self.status_message = str(exc)
            self.draw()
            return
        self.status_message = f"Setting RGB {rgb} for {self._target_label(targets)}..."
        self.draw()
        success, failures = self._apply_to_targets(
            targets, lambda bulb: self.controller.set_rgb(bulb, rgb)
        )
        if failures:
            self.status_message = self._format_failure_summary(
                "RGB set on", success, len(targets), failures
            )
        else:
            if len(targets) == 1:
                bulb = targets[0]
                self.status_message = f"{bulb.label} color {bulb.rgb}."
            else:
                self.status_message = f"Set RGB {rgb} on {success} bulbs."
        self.draw()

    def apply_scene(self) -> None:
        targets = self._targets()
        if not targets:
            return
        response = self._prompt("Scene id or name (blank to cancel): ")
        if response is None:
            self.status_message = "Scene change cancelled."
            self.draw()
            return
        try:
            scene_id = self._resolve_scene(response)
        except ValueError as exc:
            self.status_message = str(exc)
            self.draw()
            return
        self._apply_scene_to_selected(scene_id)

    def _apply_scene_to_selected(self, scene_id: int) -> None:
        targets = self._targets()
        if not targets:
            return
        self.status_message = f"Applying scene {scene_id} to {self._target_label(targets)}..."
        self.draw()
        success, failures = self._apply_to_targets(
            targets, lambda bulb: self.controller.set_scene(bulb, scene_id)
        )
        scene_name = SCENES.get(scene_id)
        if failures:
            label = f"Scene {scene_id}" if not scene_name else f"Scene {scene_id}:{scene_name}"
            self.status_message = self._format_failure_summary(label, success, len(targets), failures)
        else:
            if len(targets) == 1:
                bulb = targets[0]
                if scene_name:
                    self.status_message = f"{bulb.label} scene {scene_id}:{scene_name}."
                else:
                    self.status_message = f"{bulb.label} scene {scene_id}."
            else:
                if scene_name:
                    self.status_message = f"Applied scene {scene_id}:{scene_name} to {success} bulbs."
                else:
                    self.status_message = f"Applied scene {scene_id} to {success} bulbs."
        self.draw()

    def _move_selection(self, delta: int) -> None:
        if not self.bulbs:
            self.selected_index = 0
            return
        self.selected_index = (self.selected_index + delta) % len(self.bulbs)

    def draw(self) -> None:
        self.stdscr.erase()
        height, width = self.stdscr.getmaxyx()
        header = " pywizlight TUI"
        if self.group_mode:
            header += " [GROUP]"
        if self.show_scene_list:
            header += " [SCENES]"
        header += "  r:scan space:toggle o:on f:off b:bri c:rgb n:scene g:group l:list enter:apply s:status q:quit "
        self._safe_add(0, 0, header, width, self.attr_header)
        if self.show_scene_list:
            self._draw_scene_list(height, width)
            return
        for idx, bulb in enumerate(self.bulbs):
            y = idx + 2
            if y >= height - 1:
                break
            attr = self.attr_row_selected if idx == self.selected_index else self._row_attr(bulb, idx)
            line = self._format_bulb_line(bulb)
            self._safe_add(y, 0, line, width, attr)
        footer = self.status_message
        self._safe_add(height - 1, 0, footer, width, self._footer_attr())
        try:
            self.stdscr.refresh()
        except curses.error:  # pragma: no cover - terminal dependent
            pass

    def _handle_scene_list_key(self, key: int) -> bool:
        if key in (ord("l"), ord("L")):
            self.toggle_scene_list()
            return True
        scene_items = self._scene_items()
        if not scene_items:
            return False
        if key in (curses.KEY_UP, ord("k")):
            self.scene_list_index = (self.scene_list_index - 1) % len(scene_items)
            self.status_message = self._scene_list_status()
            return True
        if key in (curses.KEY_DOWN, ord("j")):
            self.scene_list_index = (self.scene_list_index + 1) % len(scene_items)
            self.status_message = self._scene_list_status()
            return True
        if key in (
            curses.KEY_ENTER,
            ord("\n"),
            ord("\r"),
            ord("n"),
            ord("N"),
            ord(" "),
        ):
            self._apply_scene_from_list()
            return True
        return False

    def toggle_group_mode(self) -> None:
        if not self.bulbs:
            self.group_mode = False
            self.status_message = "No bulbs available. Press r to rescan."
            self.draw()
            return
        self.group_mode = not self.group_mode
        state = "enabled" if self.group_mode else "disabled"
        current_index = min(self.selected_index, len(self.bulbs) - 1)
        self.selected_index = current_index
        if self.group_mode:
            label = self._target_label(self.bulbs)
        else:
            label = self._target_label([self.bulbs[self.selected_index]])
        self.status_message = f"Group mode {state}. Target: {label}."
        self.draw()

    def toggle_scene_list(self) -> None:
        self.show_scene_list = not getattr(self, "show_scene_list", False)
        scene_items = self._scene_items()
        if self.show_scene_list:
            if scene_items:
                self.scene_list_index = min(self.scene_list_index, len(scene_items) - 1)
            else:
                self.scene_list_index = 0
            self.status_message = self._scene_list_status()
        else:
            self.status_message = "Scene list hidden."

    def _draw_scene_list(self, height: int, width: int) -> None:
        scene_items = self._scene_items()
        available_rows = max(0, height - 2)
        if available_rows <= 0:
            return
        total = len(scene_items)
        if total == 0:
            dim_attr = self.attr_scene_hint
            self._safe_add(1, 0, "No scenes available.", width, dim_attr)
            footer = self.status_message or "Scene list"
            self._safe_add(height - 1, 0, footer, width, self._footer_attr())
            return
        self.scene_list_index = max(0, min(self.scene_list_index, total - 1))
        start = 0
        if total > available_rows:
            start = min(max(0, self.scene_list_index - available_rows // 2), total - available_rows)
        end = min(total, start + available_rows)
        row = 1
        dim_attr = self.attr_scene_hint
        if start > 0 and row < height - 1:
            self._safe_add(row, 0, "... earlier scenes ...", width, dim_attr)
            row += 1
        for idx in range(start, end):
            if row >= height - 1:
                break
            scene_id, name = scene_items[idx]
            attr = self.attr_scene_selected if idx == self.scene_list_index else self.attr_scene_row
            line = f"{scene_id:>4}: {name}"
            self._safe_add(row, 0, line, width, attr)
            row += 1
        if end < total and row < height - 1:
            self._safe_add(row, 0, "... more scenes ...", width, dim_attr)
        footer = self.status_message or "Scene list"
        self._safe_add(height - 1, 0, footer, width, self._footer_attr())

    def _scene_list_status(self) -> str:
        scene_items = self._scene_items()
        if not scene_items:
            return "Scene list (empty)."
        scene_id, name = scene_items[self.scene_list_index]
        return f"Scene list (Enter to apply) - {scene_id}: {name}"

    def _apply_scene_from_list(self) -> None:
        if not self._has_selection():
            return
        scene_items = self._scene_items()
        if not scene_items:
            self.status_message = "Scene list (empty)."
            self.draw()
            return
        scene_id, _ = scene_items[self.scene_list_index]
        self._apply_scene_to_selected(scene_id)

    def _row_attr(self, bulb: BulbInfo, index: int) -> int:
        base = self.attr_row_alt if index % 2 else self.attr_row
        if bulb.state is True:
            base |= self.attr_row_on
        elif bulb.state is False:
            base |= self.attr_row_off
        return base

    def _footer_attr(self) -> int:
        message = (self.status_message or "").lower()
        if any(word in message for word in ("error", "fail", "timeout", "invalid")):
            return self.attr_footer_error
        if any(word in message for word in ("scene list", "brightness", "color", "rgb", "scene", "group")):
            return self.attr_footer_info
        return self.attr_footer

    def _targets(self) -> List[BulbInfo]:
        if not self._has_selection():
            return []
        return list(self.bulbs) if self.group_mode else [self.bulbs[self.selected_index]]

    def _target_label(self, bulbs: List[BulbInfo]) -> str:
        if not bulbs:
            return "no bulbs"
        if len(bulbs) == 1:
            return bulbs[0].label
        return f"{len(bulbs)} bulbs"

    def _apply_to_targets(
        self,
        targets: List[BulbInfo],
        executor: Callable[[BulbInfo], Dict[str, Optional[Any]]],
    ) -> Tuple[int, List[Tuple[BulbInfo, str]]]:
        success = 0
        failures: List[Tuple[BulbInfo, str]] = []
        for bulb in targets:
            try:
                result = executor(bulb)
            except Exception as exc:  # pragma: no cover - network dependent
                failures.append((bulb, str(exc)))
                continue
            self._apply_result(bulb, result)
            error = result.get("error")
            if error:
                failures.append((bulb, str(error)))
            else:
                success += 1
        return success, failures

    def _format_failure_summary(
        self,
        action: str,
        success: int,
        total: int,
        failures: List[Tuple[BulbInfo, str]],
    ) -> str:
        detail = "; ".join(f"{bulb.label}: {err}" for bulb, err in failures[:2])
        if len(failures) > 2:
            detail += "; ..."
        return f"{action} {success}/{total} bulbs (failures: {detail})"

    def _scene_items(self) -> List[Tuple[int, str]]:
        return sorted(SCENES.items())

    def _safe_add(self, y: int, x: int, text: str, width: int, attr: int) -> None:
        if y < 0 or y >= self.stdscr.getmaxyx()[0] or width <= 0:
            return
        try:
            self.stdscr.addnstr(y, x, text.ljust(width), width, attr)
        except curses.error:  # pragma: no cover - terminal dependent
            pass

    def _format_bulb_line(self, bulb: BulbInfo) -> str:
        if bulb.state is True:
            state = "ON "
        elif bulb.state is False:
            state = "OFF"
        else:
            state = "???"
        parts = [f"[{state}] {bulb.label} ({bulb.ip})"]
        details: List[str] = []
        if bulb.scene_id is not None:
            scene_name = SCENES.get(bulb.scene_id)
            if scene_name:
                details.append(f"scene={bulb.scene_id}:{scene_name}")
            else:
                details.append(f"scene={bulb.scene_id}")
        if bulb.brightness is not None:
            details.append(f"bri={bulb.brightness}")
        if bulb.rgb:
            details.append(f"rgb={bulb.rgb[0]},{bulb.rgb[1]},{bulb.rgb[2]}")
        if bulb.last_error:
            details.append(f"! {bulb.last_error}")
        if details:
            parts.append("  ".join(details))
        return "  ".join(parts)

    def _apply_result(self, bulb: BulbInfo, result: Dict[str, Optional[Any]]) -> None:
        if result.get("state") is not None:
            bulb.state = bool(result["state"])
        if result.get("brightness") is not None:
            bulb.brightness = int(result["brightness"])  # type: ignore[arg-type]
        if result.get("rgb"):
            rgb_val = result["rgb"]
            bulb.rgb = (
                int(rgb_val[0]),
                int(rgb_val[1]),
                int(rgb_val[2]),
            )  # type: ignore[index]
        if result.get("scene_id") is not None:
            bulb.scene_id = int(result["scene_id"])  # type: ignore[arg-type]
        if result.get("mac"):
            bulb.mac = result["mac"]
        bulb.last_error = result.get("error")

    def _prompt(self, prompt: str) -> Optional[str]:
        height, width = self.stdscr.getmaxyx()
        try:
            self.stdscr.move(height - 1, 0)
            self.stdscr.clrtoeol()
        except curses.error:  # pragma: no cover - terminal dependent
            pass
        self._safe_add(height - 1, 0, prompt, width, curses.A_BOLD)
        try:
            curses.echo()
            self.stdscr.refresh()
        except curses.error:  # pragma: no cover - terminal dependent
            pass
        try:
            raw = self.stdscr.getstr(height - 1, len(prompt), max(1, width - len(prompt) - 1))
        except curses.error:  # pragma: no cover - terminal dependent
            raw = b""
        finally:
            try:
                curses.noecho()
            except curses.error:  # pragma: no cover - terminal dependent
                pass
        text = raw.decode(errors="ignore").strip()
        if not text:
            return None
        return text

    def _parse_rgb_input(self, text: str) -> RGBTuple:
        value = text.strip()
        if not value:
            raise ValueError("RGB value is required.")
        if value.startswith("#"):
            hex_value = value[1:]
            if len(hex_value) != 6 or not all(c in "0123456789abcdefABCDEF" for c in hex_value):
                raise ValueError("RGB hex must be in the form #RRGGBB.")
            return tuple(int(hex_value[i : i + 2], 16) for i in (0, 2, 4))  # type: ignore[return-value]
        parts = [part for part in re.split(r"[ ,]+", value) if part]
        if len(parts) != 3:
            raise ValueError("RGB must have three components (e.g. 255,128,0).")
        try:
            rgb = tuple(int(part) for part in parts)
        except ValueError:
            raise ValueError("RGB components must be integers.") from None
        if any(component < 0 or component > 255 for component in rgb):
            raise ValueError("RGB components must be between 0 and 255.")
        return rgb  # type: ignore[return-value]

    def _resolve_scene(self, token: str) -> int:
        text = token.strip()
        if not text:
            raise ValueError("Scene id or name is required.")
        if text.isdigit():
            return int(text)
        lower = text.lower()
        exact_matches = [scene_id for scene_id, name in SCENES.items() if name.lower() == lower]
        if exact_matches:
            return exact_matches[0]
        partial_matches = [scene_id for scene_id, name in SCENES.items() if lower in name.lower()]
        if partial_matches:
            return partial_matches[0]
        raise ValueError(f"Unknown scene '{token}'.")

    def _has_selection(self) -> bool:
        if not self.bulbs:
            self.status_message = "No bulbs available. Press r to rescan."
            self.draw()
            return False
        return True

    def _format_status_summary(self, bulb: BulbInfo) -> str:
        if bulb.state is True:
            status = "on"
        elif bulb.state is False:
            status = "off"
        else:
            status = "unknown"
        extras: List[str] = []
        if bulb.brightness is not None:
            extras.append(f"bri {bulb.brightness}")
        if bulb.rgb:
            extras.append(f"rgb {bulb.rgb[0]},{bulb.rgb[1]},{bulb.rgb[2]}")
        if bulb.scene_id is not None:
            scene_name = SCENES.get(bulb.scene_id)
            if scene_name:
                extras.append(f"scene {bulb.scene_id}:{scene_name}")
            else:
                extras.append(f"scene {bulb.scene_id}")
        if extras:
            return f"{status} ({', '.join(extras)})"
        return status


def run_tui(broadcast_address: str = "255.255.255.255", wait_time: float = 5.0) -> None:
    """Launch the wizlight TUI."""

    if curses is None:
        raise RuntimeError(f"curses is required for the TUI: {_CURSES_IMPORT_ERROR}")

    controller = WizAsyncController(broadcast_address=broadcast_address, wait_time=wait_time)

    def _wrapped(stdscr: "curses._CursesWindow") -> None:
        WizTUI(stdscr, controller).run()

    try:
        curses.wrapper(_wrapped)
    finally:
        controller.shutdown()
