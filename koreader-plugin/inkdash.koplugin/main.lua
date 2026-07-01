--[[--
Ink Dashboard plugin for KOReader.

Fetches a PNG dashboard from a companion backend and displays it full-screen.
The backend owns data collection and image rendering; this plugin owns device
integration, scheduling, Wi-Fi lifecycle, and e-ink refresh.
]]

local DataStorage = require("datastorage")
local Device = require("device")
local Dispatcher = require("dispatcher")
local Blitbuffer = require("ffi/blitbuffer")
local Font = require("ui/font")
local Geom = require("ui/geometry")
local GestureRange = require("ui/gesturerange")
local ImageWidget = require("ui/widget/imagewidget")
local InfoMessage = require("ui/widget/infomessage")
local InputContainer = require("ui/widget/container/inputcontainer")
local LuaSettings = require("luasettings")
local MultiInputDialog = require("ui/widget/multiinputdialog")
local NetworkMgr = require("ui/network/manager")
local RenderImage = require("ui/renderimage")
local Screen = Device.screen
local TextWidget = require("ui/widget/textwidget")
local UIManager = require("ui/uimanager")
local WidgetContainer = require("ui/widget/container/widgetcontainer")
local logger = require("logger")
local util = require("util")
local _ = require("gettext")
local T = require("ffi/util").template

local function dashboardColor(hex, gray_value, fallback)
    if Blitbuffer.colorFromString then
        local ok, color = pcall(function()
            return Blitbuffer.colorFromString(hex)
        end)
        if ok and color then
            return color
        end
    end
    if Blitbuffer.Color8 then
        local ok, color = pcall(function()
            return Blitbuffer.Color8(gray_value)
        end)
        if ok and color then
            return color
        end
    end
    return fallback
end

-- Match server/internal/render/renderer.go so the local clock blends into the PNG.
local CLOCK_BACKGROUND_COLOR = dashboardColor("#f8f8f3", 0xF8, Blitbuffer.COLOR_WHITE)
local CLOCK_TEXT_COLOR = dashboardColor("#111111", 0x11, Blitbuffer.COLOR_BLACK)

local function isChineseLanguage(language)
    language = tostring(language or ""):lower()
    return language:match("^zh") ~= nil
end

local function zhWeekday(wday)
    return ({ "周日", "周一", "周二", "周三", "周四", "周五", "周六" })[wday] or ""
end

local LocalClockOverlay = WidgetContainer:extend {
    screen_width = nil,
    screen_height = nil,
    orientation = "auto",
    language = "en",
    updated_at = nil,
    clock_widget = nil,
    rendered_minute = nil,
}

function LocalClockOverlay:init()
    self.screen_width = self.screen_width or Screen:getWidth()
    self.screen_height = self.screen_height or Screen:getHeight()
    self.dimen = Geom:new { x = 0, y = 0, w = self.screen_width, h = self.screen_height }
end

function LocalClockOverlay:getSize()
    return self.dimen
end

function LocalClockOverlay:minuteKey()
    return os.date("%Y%m%d%H%M")
end

function LocalClockOverlay:clockText()
    local now = os.date("*t")
    local time_text = string.format("%02d:%02d", now.hour, now.min)
    local date_text
    if isChineseLanguage(self.language) then
        date_text = string.format("%d月%02d日 %s", now.month, now.day, zhWeekday(now.wday))
        if self.updated_at and self.updated_at ~= "" then
            date_text = date_text .. " · 更新 " .. self.updated_at
        end
        return time_text, date_text
    end
    date_text = os.date("%a, %b %d")
    if self.updated_at and self.updated_at ~= "" then
        date_text = date_text .. " · Updated " .. self.updated_at
    end
    return time_text, date_text
end

function LocalClockOverlay:layout()
    local layout_w = self.orientation == "rotated" and self.screen_height or self.screen_width
    local padding = layout_w < 700 and 24 or 28
    local source_w = 390
    local source_h = 96
    if self.orientation == "rotated" then
        return {
            source_w = source_w,
            source_h = source_h,
            screen_x = self.screen_width - (padding - 6) - source_h,
            screen_y = padding - 6,
            screen_w = source_h,
            screen_h = source_w,
            text_x = 6,
            time_y = 2,
            date_y = 64,
            rotation_angle = 270,
            rotated = true,
        }
    end
    return {
        source_w = source_w,
        source_h = source_h,
        screen_x = padding - 6,
        screen_y = padding - 6,
        screen_w = source_w,
        screen_h = source_h,
        text_x = 6,
        time_y = 2,
        date_y = 64,
        rotation_angle = 0,
    }
end

function LocalClockOverlay:ensureClockWidget()
    local minute = self:minuteKey()
    if self.clock_widget and self.rendered_minute == minute then
        return true
    end

    local time_text, date_text = self:clockText()
    if self.clock_widget then
        self.clock_widget:free()
    end

    local layout = self:layout()
    local clock_bb = Blitbuffer.new(layout.source_w, layout.source_h, Screen.bb:getType())
    clock_bb:fill(CLOCK_BACKGROUND_COLOR)

    self.rendered_minute = minute
    local time_widget = TextWidget:new {
        text = time_text,
        face = Font:getFace("cfont", 52),
        bold = true,
        fgcolor = CLOCK_TEXT_COLOR,
        max_width = layout.source_w - 12,
    }
    local date_widget = TextWidget:new {
        text = date_text,
        face = Font:getFace("cfont", 16),
        fgcolor = CLOCK_TEXT_COLOR,
        max_width = layout.source_w - 12,
    }

    time_widget:paintTo(clock_bb, layout.text_x, layout.time_y)
    date_widget:paintTo(clock_bb, layout.text_x + 2, layout.date_y)
    time_widget:free()
    date_widget:free()

    self.clock_widget = ImageWidget:new {
        image = clock_bb,
        image_disposable = true,
        rotation_angle = layout.rotation_angle,
        scale_factor = 1,
    }
    return true
end

function LocalClockOverlay:invalidate()
    self.rendered_minute = nil
    if self.clock_widget then
        self.clock_widget:free()
        self.clock_widget = nil
    end
end

function LocalClockOverlay:paintTo(bb, x, y)
    if not self:ensureClockWidget() then
        return
    end
    local layout = self:layout()
    self.clock_widget:paintTo(bb, x + layout.screen_x, y + layout.screen_y)
end

function LocalClockOverlay:free()
    self:invalidate()
end

function LocalClockOverlay:onCloseWidget()
    self:free()
end

local DashboardOverlay = WidgetContainer:extend {
    image_widget = nil,
    clock_overlay = nil,
    dimen = nil,
}

function DashboardOverlay:getSize()
    return self.dimen
end

function DashboardOverlay:paintTo(bb, x, y)
    if self.image_widget then
        self.image_widget:paintTo(bb, x, y)
    end
    if self.clock_overlay then
        self.clock_overlay:paintTo(bb, x, y)
    end
end

function DashboardOverlay:free()
    if self.image_widget and self.image_widget.free then
        self.image_widget:free()
    end
    if self.clock_overlay and self.clock_overlay.free then
        self.clock_overlay:free()
    end
end

function DashboardOverlay:onCloseWidget()
    self:free()
end

local InkDashboard = WidgetContainer:extend {
    name = "inkdash",
    is_doc_only = false,

    settings = nil,
    settings_file = nil,
    refresh_task = nil,
    auto_refresh_enabled = false,
    auto_refresh_scheduled = false,
    interactive_mode = false,
    image_widget = nil,
    local_clock_overlay = nil,
    local_clock_task = nil,
    last_image_path = nil,
    last_image_filename = nil,
    last_image_download_timestamp = 0,
    last_fetch_timestamp = 0,
    retry_count = 0,
}

InkDashboard.default_settings = {
    api_key = nil,
    base_url = "http://127.0.0.1:8787",
    refresh_interval = 600,
    use_server_refresh_rate = true,
    refresh_type = "ui",
    orientation = "auto",
    show_notifications = true,
    local_clock = true,
    local_clock_language = "auto",
    local_clock_live_minutes = false,
    image_cache_seconds = -1,
    user_agent = "ink-dashboard-koreader/0.1.0",
}

InkDashboard.constants = {
    api_key_file = "apikey.txt",
    config_file = "config.lua",
    default_filename = "inkdash.png",
    debounce_seconds = 20,
    retry_base_seconds = 60,
}

function InkDashboard:init()
    self.settings_file = LuaSettings:open(DataStorage:getSettingsDir() .. "/inkdash.lua")
    local defaults = util.tableDeepCopy(self.default_settings)
    local file_defaults = self:loadConfigFromFile()
    if file_defaults then
        for key, value in pairs(file_defaults) do
            if defaults[key] ~= nil then
                defaults[key] = value
            end
        end
    end
    self.settings = defaults
    local saved_settings = self.settings_file:readSetting("settings")
    if saved_settings then
        for key, value in pairs(saved_settings) do
            self.settings[key] = value
        end
    end
    self.auto_refresh_enabled = self.settings_file:readSetting("auto_refresh_enabled") or false
    if saved_settings then
        self:migrateSavedPowerDefaults()
    end

    local file_api_key = self:loadApiKeyFromFile()
    if file_api_key then
        self.settings.api_key = file_api_key
        self:saveSettings()
    end

    self.refresh_task = function()
        self:fetchAndDisplay()
    end
    self.local_clock_task = function()
        self:refreshLocalClock()
    end

    Dispatcher:registerAction("inkdash_fetch_now", {
        category = "none",
        event = "InkDashFetch",
        title = _("Ink Dashboard: Fetch now"),
        general = true,
    })
    Dispatcher:registerAction("inkdash_start", {
        category = "none",
        event = "InkDashStart",
        title = _("Ink Dashboard: Start"),
        general = true,
    })

    self.ui.menu:registerToMainMenu(self)

    if self.auto_refresh_enabled then
        self:startAutoRefresh()
    end
end

function InkDashboard:onFlushSettings()
    if self.settings_file then
        self.settings_file:saveSetting("settings", self.settings)
        self.settings_file:saveSetting("auto_refresh_enabled", self.auto_refresh_enabled)
        self.settings_file:flush()
    end
end

function InkDashboard:saveSettings()
    self:onFlushSettings()
end

function InkDashboard:migrateSavedPowerDefaults()
    if self.settings_file:readSetting("power_defaults_migrated") then
        return
    end

    if tonumber(self.settings.image_cache_seconds) == 0 then
        self.settings.image_cache_seconds = -1
        self.settings_file:saveSetting("settings", self.settings)
    end
    self.settings_file:saveSetting("power_defaults_migrated", true)
    self.settings_file:flush()
end

function InkDashboard:getPluginDir()
    local info = debug.getinfo(1, "S")
    local filepath = info.source:match("^@(.+)$")
    if filepath then
        return filepath:match("(.*/)")
    end
    return nil
end

function InkDashboard:loadApiKeyFromFile()
    local plugin_dir = self:getPluginDir()
    if not plugin_dir then
        return nil
    end

    local file = io.open(plugin_dir .. self.constants.api_key_file, "r")
    if not file then
        return nil
    end

    local content = file:read("*all")
    file:close()

    local api_key = content and content:match("^%s*(.-)%s*$")
    if api_key and api_key ~= "" then
        logger.info("InkDash: API key loaded from file")
        return api_key
    end
    return nil
end

function InkDashboard:loadConfigFromFile()
    local plugin_dir = self:getPluginDir()
    if not plugin_dir then
        return nil
    end

    local config_path = plugin_dir .. self.constants.config_file
    local chunk, err = loadfile(config_path)
    if not chunk then
        logger.dbg("InkDash: No config.lua loaded:", err)
        return nil
    end

    local ok, config = pcall(chunk)
    if not ok then
        logger.err("InkDash: Failed to load config.lua:", config)
        return nil
    end
    if type(config) ~= "table" then
        logger.err("InkDash: config.lua must return a table")
        return nil
    end

    logger.info("InkDash: defaults loaded from config.lua")
    return config
end

function InkDashboard:notify(message, opts)
    opts = opts or {}
    local msg_type = opts.type or "info"
    local context = opts.context

    logger.info("InkDash [" .. msg_type .. "]: " .. message)
    if context then
        logger.dbg("InkDash context: " .. context)
    end

    if not self.settings.show_notifications and msg_type ~= "error" and not opts.force then
        return
    end

    local timeout = opts.timeout
    if not timeout then
        timeout = msg_type == "error" and 5 or 2
    end

    local full_message = _(message)
    if context and context ~= "" then
        full_message = full_message .. "\n\n" .. context
    end

    UIManager:show(InfoMessage:new {
        text = full_message,
        timeout = timeout,
    })
end

function InkDashboard:showError(message, context)
    self:notify(message, { type = "error", context = context })
end

function InkDashboard:showInfo(message)
    self:notify(message, { type = "info" })
end

function InkDashboard:showProgress(message, context)
    self:notify(message, { type = "progress", context = context, timeout = 2 })
end

function InkDashboard:fetchDisplayMetadata()
    if not self.settings.api_key or self.settings.api_key == "" then
        self:showError("Please configure your Ink Dashboard API key first.")
        return nil
    end

    local http = require("socket.http")
    local https = require("ssl.https")
    local ltn12 = require("ltn12")
    local JSON = require("json")

    local base_url = (self.settings.base_url or ""):gsub("/+$", "")
    if base_url == "" then
        self:showError("Please configure your Ink Dashboard base URL first.")
        return nil
    end

    local request_url = base_url .. "/api/display"
    local sink = {}
    local battery = "0"
    if Device:hasBattery() then
        local powerd = Device:getPowerDevice()
        battery = tostring(powerd:getCapacity())
    end

    local request = {
        url = request_url,
        method = "GET",
        headers = {
            ["access-token"] = self.settings.api_key,
            ["percent-charged"] = battery,
            ["png-width"] = tostring(Screen:getWidth()),
            ["png-height"] = tostring(Screen:getHeight()),
            ["orientation"] = self.settings.orientation or "auto",
            ["local-clock"] = self.settings.local_clock and "true" or "false",
            ["rssi"] = "0",
            ["User-Agent"] = self.settings.user_agent,
        },
        sink = ltn12.sink.table(sink),
        protocol = "any",
        options = { "all", "no_sslv2", "no_sslv3" },
        verify = "none",
    }

    logger.info("InkDash: Fetching metadata from", request_url)
    local httpx = request_url:match("^https://") and https or http
    local success_code, status_code = httpx.request(request)
    if not success_code or success_code ~= 1 then
        self:showError("Failed to reach Ink Dashboard server", tostring(success_code))
        return nil
    end
    if status_code ~= 200 then
        self:showError("Ink Dashboard API request failed", "HTTP " .. tostring(status_code))
        return nil
    end

    local ok, response = pcall(JSON.decode, table.concat(sink))
    if not ok or not response then
        self:showError("Ink Dashboard API returned invalid JSON.")
        return nil
    end
    return response
end

function InkDashboard:downloadImage(image_url, filepath)
	local http = require("socket.http")
	local https = require("ssl.https")
	local ltn12 = require("ltn12")

	local temp_path = filepath .. ".part"
	local file = io.open(temp_path, "wb")
	if not file then
		logger.err("InkDash: Failed to open file for writing:", temp_path)
		return false
	end

    local request = {
        url = image_url,
        sink = ltn12.sink.file(file),
        headers = {
            ["User-Agent"] = self.settings.user_agent,
        },
        protocol = "any",
        options = { "all", "no_sslv2", "no_sslv3" },
        verify = "none",
    }

	logger.info("InkDash: Downloading image from", image_url)
	local httpx = image_url:match("^https://") and https or http
	local success_code, status_code = httpx.request(request)
	if not success_code or success_code ~= 1 or status_code ~= 200 then
		os.remove(temp_path)
		logger.err("InkDash: Image download failed:", success_code, status_code)
		return false
	end
	if os.rename(temp_path, filepath) ~= true then
		os.remove(temp_path)
		logger.err("InkDash: Failed to install downloaded image:", filepath)
		return false
	end
	return true
end

function InkDashboard:fileExists(filepath)
	local file = io.open(filepath, "rb")
	if not file then
		return false
	end
	file:close()
	return true
end

function InkDashboard:cachedImageStillFresh(filename, image_path)
	if filename ~= self.last_image_filename then
		return false
	end
	if not self:fileExists(image_path) then
		return false
	end
	local cache_seconds = tonumber(self.settings.image_cache_seconds) or -1
	if cache_seconds < 0 then
		return true
	end
	if cache_seconds == 0 then
		return false
	end
	local now = UIManager:getElapsedTimeSinceBoot()
	return now - self.last_image_download_timestamp < cache_seconds
end

function InkDashboard:downloadImageIfNeeded(response)
	if not response.image_url or response.image_url == "" then
		return nil
    end

    local filename = response.filename or self.constants.default_filename
    if not filename:match("%.png$") then
        filename = filename .. ".png"
	end

	local image_path = DataStorage:getDataDir() .. "/" .. filename
	if self:cachedImageStillFresh(filename, image_path) then
		logger.info("InkDash: Image unchanged, using fresh cached file:", image_path)
		return image_path
	end
	if filename == self.last_image_filename then
		logger.info("InkDash: Image filename unchanged, refreshing cached file:", filename)
	end

	self:showProgress("Downloading dashboard...", filename)
	if not self:downloadImage(response.image_url, image_path) then
        return nil
    end

    if self.last_image_path and self.last_image_path ~= image_path then
        os.remove(self.last_image_path)
    end

	self.last_image_filename = filename
	self.last_image_path = image_path
	self.last_image_download_timestamp = UIManager:getElapsedTimeSinceBoot()
	return image_path
end

function InkDashboard:shouldUseLocalClock(response)
    return self.settings.local_clock and response and response.local_clock == true
end

function InkDashboard:localClockLanguage(response)
    local configured = self.settings.local_clock_language or "auto"
    if configured ~= "auto" then
        return configured
    end
    if response and response.language and response.language ~= "" then
        return response.language
    end
    return "en"
end

function InkDashboard:secondsUntilNextMinute()
    local now = os.date("*t")
    local delay = 60 - (tonumber(now.sec) or 0)
    if delay < 1 then
        delay = 60
    end
    return delay
end

function InkDashboard:localClockRefreshRegion()
    if self.local_clock_overlay then
        local layout = self.local_clock_overlay:layout()
        return Geom:new { x = layout.screen_x, y = layout.screen_y, w = layout.screen_w, h = layout.screen_h }
    end
    return Geom:new { x = 0, y = 0, w = 330, h = 124 }
end

function InkDashboard:unscheduleLocalClock()
    if self.local_clock_task then
        UIManager:unschedule(self.local_clock_task)
    end
end

function InkDashboard:scheduleLocalClock()
    if not self.image_widget or not self.local_clock_overlay then
        return
    end
    if not self.settings.local_clock_live_minutes then
        return
    end
    self:unscheduleLocalClock()
    UIManager:scheduleIn(self:secondsUntilNextMinute(), self.local_clock_task)
end

function InkDashboard:startLocalClock()
    if not self.local_clock_overlay then
        return
    end
    self:scheduleLocalClock()
end

function InkDashboard:refreshLocalClock()
    if not self.image_widget or not self.local_clock_overlay then
        return
    end
    self.local_clock_overlay:invalidate()
    UIManager:setDirty(self.image_widget, self.settings.refresh_type or "ui", self:localClockRefreshRegion())
    self:scheduleLocalClock()
end

function InkDashboard:stopLocalClock()
    self:unscheduleLocalClock()
    if self.local_clock_overlay then
        self.local_clock_overlay:free()
        self.local_clock_overlay = nil
    end
end

function InkDashboard:displayImage(image_path, response)
    local screen_width = Screen:getWidth()
    local screen_height = Screen:getHeight()

    local image_bb = RenderImage:renderImageFile(image_path, true, screen_width, screen_height)
    if not image_bb then
        self:showError("Failed to render dashboard image.")
        return false
    end

    if self.image_widget then
        self:stopLocalClock()
        UIManager:close(self.image_widget)
        self.image_widget = nil
    end

    local image = ImageWidget:new {
        image = image_bb,
        image_disposable = true,
        width = screen_width,
        height = screen_height,
        alpha = true,
    }

    local content
    if self:shouldUseLocalClock(response) then
        self.local_clock_overlay = LocalClockOverlay:new {
            screen_width = screen_width,
            screen_height = screen_height,
            orientation = self.settings.orientation or "auto",
            language = self:localClockLanguage(response),
            updated_at = response.updated_at,
        }
        logger.info("InkDash: Local clock overlay enabled via KOReader TextWidget")
        content = DashboardOverlay:new {
            dimen = Geom:new { x = 0, y = 0, w = screen_width, h = screen_height },
            image_widget = image,
            clock_overlay = self.local_clock_overlay,
        }
    else
        self.local_clock_overlay = nil
        content = image
    end

    self.image_widget = InputContainer:new {
        dimen = { x = 0, y = 0, w = screen_width, h = screen_height },
        content,
    }

    self.image_widget.onTapClose = function()
        logger.info("InkDash: Closing dashboard via tap")
        if self.interactive_mode then
            self.interactive_mode = false
            self:stopAutoRefresh()
        end
        self:stopLocalClock()
        UIManager:close(self.image_widget)
        self.image_widget = nil
        return true
    end

    if Device:isTouchDevice() then
        self.image_widget.ges_events = {
            TapClose = {
                GestureRange:new {
                    ges = "tap",
                    range = Geom:new { x = 0, y = 0, w = screen_width, h = screen_height },
                },
            },
        }
    end

    UIManager:show(self.image_widget)
    UIManager:setDirty(self.image_widget, self.settings.refresh_type or "ui")
    self:startLocalClock()
    return true
end

function InkDashboard:checkDebounce(skip_debounce)
    if skip_debounce then
        return true
    end

    local now = UIManager:getElapsedTimeSinceBoot()
    if now - self.last_fetch_timestamp <= self.constants.debounce_seconds then
        return false
    end
    self.last_fetch_timestamp = now
    return true
end

function InkDashboard:fetchAndDisplay(skip_debounce)
    if not self:checkDebounce(skip_debounce) then
        return
    end

    NetworkMgr:runWhenConnected(function()
        UIManager:scheduleIn(2, function()
            self:performFetch()
        end)
    end)
end

function InkDashboard:performFetch()
    local response = self:fetchDisplayMetadata()
    if not response then
        self:handleFetchError("metadata fetch failed")
        NetworkMgr:afterWifiAction()
        return
    end

    self:updateRefreshInterval(response)

    local image_path = self:downloadImageIfNeeded(response)
    if not image_path then
        self:handleFetchError("image download failed")
        NetworkMgr:afterWifiAction()
        return
    end

    if self:displayImage(image_path, response) then
        self.retry_count = 0
        self:scheduleNextRefresh()
    else
        self:handleFetchError("image render failed")
    end

    NetworkMgr:afterWifiAction()
end

function InkDashboard:updateRefreshInterval(response)
    if not self.settings.use_server_refresh_rate then
        return
    end

    local refresh_rate = tonumber(response.refresh_rate)
    if refresh_rate and refresh_rate > 0 then
        self.settings.refresh_interval = refresh_rate
        self:saveSettings()
    end
end

function InkDashboard:handleFetchError(reason)
    self.retry_count = self.retry_count + 1
    local max_interval = self.settings.refresh_interval or 600
    local delay = math.min(self.constants.retry_base_seconds * (2 ^ (self.retry_count - 1)), max_interval)
    self:showError(T(_("Ink Dashboard failed: %1. Retrying in %2 seconds."), reason, delay))
    if self.auto_refresh_enabled then
        UIManager:scheduleIn(delay, self.refresh_task)
        self.auto_refresh_scheduled = true
    end
end

function InkDashboard:unscheduleRefreshTask()
    if self.refresh_task then
        UIManager:unschedule(self.refresh_task)
        self.auto_refresh_scheduled = false
    end
end

function InkDashboard:scheduleNextRefresh()
    if not self.auto_refresh_enabled then
        return
    end

    self:unscheduleRefreshTask()
    local interval = self.settings.refresh_interval or 600
    UIManager:scheduleIn(interval, self.refresh_task)
    self.auto_refresh_scheduled = true
    logger.info("InkDash: Next refresh in", interval, "seconds")
end

function InkDashboard:startAutoRefresh()
    self:unscheduleRefreshTask()
    self.auto_refresh_enabled = true
    self.auto_refresh_scheduled = false
    self:saveSettings()
    self:fetchAndDisplay(true)
end

function InkDashboard:stopAutoRefresh()
    self:unscheduleRefreshTask()
    self.auto_refresh_enabled = false
    self.auto_refresh_scheduled = false
    self:saveSettings()
end

function InkDashboard:onSuspend()
    self:unscheduleLocalClock()
    if self.auto_refresh_enabled then
        self:unscheduleRefreshTask()
    end
end

function InkDashboard:onResume()
    if self.local_clock_overlay then
        self:refreshLocalClock()
    end
    if self.auto_refresh_enabled then
        self:unscheduleRefreshTask()
        self:fetchAndDisplay(true)
    end
end

function InkDashboard:onCloseWidget()
    self:stopAutoRefresh()
    self:stopLocalClock()
    if self.image_widget then
        UIManager:close(self.image_widget)
        self.image_widget = nil
    end
end

function InkDashboard:showConfigDialog()
    self.config_dialog = MultiInputDialog:new {
        title = _("Configure Ink Dashboard"),
        fields = {
            {
                text = self.settings.api_key or "",
                hint = _("API key"),
                input_type = "string",
                password = true,
            },
            {
                text = self.settings.base_url or "http://127.0.0.1:8787",
                hint = _("Base URL"),
                input_type = "string",
            },
            {
                text = tostring(self.settings.refresh_interval or 600),
                hint = _("Refresh interval (seconds)"),
                input_type = "number",
            },
            {
                text = self.settings.orientation or "auto",
                hint = _("Orientation: auto, portrait, landscape, rotated"),
                input_type = "string",
            },
        },
        buttons = {
            {
                {
                    text = _("Cancel"),
                    id = "close",
                    callback = function()
                        UIManager:close(self.config_dialog)
                    end,
                },
                {
                    text = _("Save"),
                    is_enter_default = true,
                    callback = function()
                        local fields = self.config_dialog:getFields()
                        self.settings.api_key = fields[1]
                        self.settings.base_url = fields[2]
                        self.settings.refresh_interval = tonumber(fields[3]) or 600
                        self.settings.orientation = fields[4] or "auto"
                        self:saveSettings()
                        UIManager:close(self.config_dialog)
                        self:showInfo("Ink Dashboard settings saved.")
                    end,
                },
            },
        },
    }

    UIManager:show(self.config_dialog)
end

function InkDashboard:createToggleMenuItem(label_on, label_off, getter, toggler)
    return {
        text_func = function()
            return getter() and _(label_on) or _(label_off)
        end,
        callback = function()
            local message = toggler()
            if message then
                UIManager:show(InfoMessage:new { text = _(message), timeout = 2 })
            end
        end,
    }
end

function InkDashboard:createRadioMenuItem(label, setting_key, value)
    return {
        text = _(label),
        checked_func = function()
            return self.settings[setting_key] == value
        end,
        callback = function()
            self.settings[setting_key] = value
            self:saveSettings()
        end,
    }
end

function InkDashboard:addToMainMenu(menu_items)
    menu_items.inkdash = {
        text = _("Ink Dashboard"),
        sorting_hint = "tools",
        sub_item_table = {
            {
                text = _("Start dashboard"),
                callback = function()
                    self.interactive_mode = true
                    self:startAutoRefresh()
                end,
            },
            {
                text = _("Fetch dashboard now"),
                callback = function()
                    self:fetchAndDisplay(true)
                end,
            },
            {
                text = _("Configure"),
                keep_menu_open = true,
                callback = function()
                    self:showConfigDialog()
                end,
            },
            self:createToggleMenuItem(
                "Disable auto-refresh",
                "Enable auto-refresh",
                function() return self.auto_refresh_enabled end,
                function()
                    if self.auto_refresh_enabled then
                        self:stopAutoRefresh()
                        return "Auto-refresh disabled."
                    end
                    self:startAutoRefresh()
                    return "Auto-refresh enabled."
                end
            ),
            self:createToggleMenuItem(
                "Use manual refresh interval",
                "Use server refresh interval",
                function() return self.settings.use_server_refresh_rate end,
                function()
                    self.settings.use_server_refresh_rate = not self.settings.use_server_refresh_rate
                    self:saveSettings()
                    return self.settings.use_server_refresh_rate
                        and "Server refresh interval enabled."
                        or "Manual refresh interval enabled."
                end
            ),
            self:createToggleMenuItem(
                "Hide status notifications",
                "Show status notifications",
                function() return self.settings.show_notifications end,
                function()
                    self.settings.show_notifications = not self.settings.show_notifications
                    self:saveSettings()
                    return self.settings.show_notifications
                        and "Status notifications enabled."
                        or "Status notifications hidden."
                end
            ),
            self:createToggleMenuItem(
                "Use server-rendered clock",
                "Use KOReader local clock",
                function() return self.settings.local_clock end,
                function()
                    self.settings.local_clock = not self.settings.local_clock
                    self:saveSettings()
                    if not self.settings.local_clock then
                        self:stopLocalClock()
                    end
                    self:fetchAndDisplay(true)
                    return self.settings.local_clock
                        and "KOReader local clock enabled."
                        or "Server-rendered clock enabled."
                end
            ),
            self:createToggleMenuItem(
                "Disable live minute clock",
                "Enable live minute clock",
                function() return self.settings.local_clock_live_minutes end,
                function()
                    self.settings.local_clock_live_minutes = not self.settings.local_clock_live_minutes
                    self:saveSettings()
                    if self.settings.local_clock_live_minutes then
                        self:startLocalClock()
                    else
                        self:unscheduleLocalClock()
                    end
                    return self.settings.local_clock_live_minutes
                        and "Live minute clock enabled."
                        or "Live minute clock disabled."
                end
            ),
            {
                text = _("E-ink refresh type"),
                sub_item_table = {
                    self:createRadioMenuItem("UI (balanced)", "refresh_type", "ui"),
                    self:createRadioMenuItem("Full (best quality)", "refresh_type", "full"),
                    self:createRadioMenuItem("Flash UI", "refresh_type", "flashui"),
                    self:createRadioMenuItem("Partial (fastest)", "refresh_type", "partial"),
                },
            },
        },
    }
end

function InkDashboard:onInkDashFetch()
    self:fetchAndDisplay(true)
end

function InkDashboard:onInkDashStart()
    self.interactive_mode = true
    self:startAutoRefresh()
end

return InkDashboard
