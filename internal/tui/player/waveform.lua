-- A single decoder supplies both audible music and a tiny offscreen waveform.
-- The null video output never creates a window or writes to the terminal.
local mp = require 'mp'
local ready = not mp.get_property_bool('idle-active', true)
local configured, failed = false, false
local serial = 0
local width, height = 256, 128

local function clear()
    configured, failed = false, false
    mp.set_property_native('user-data/plexctl-waveform', {valid=false})
end

mp.register_event('start-file', function() ready=false; clear() end)
mp.register_event('file-loaded', function() ready=true end)
mp.register_event('end-file', function() ready=false; clear() end)
mp.register_event('seek', function()
    mp.set_property_native('user-data/plexctl-waveform', {valid=false})
end)

mp.add_periodic_timer(0.05, function()
    if not ready or failed or not mp.get_property_bool('user-data/plex-audio-only', false) then return end
    if not configured then
        local aid = mp.get_property_number('aid')
        if not aid or aid < 1 then return end
        -- Only the visualization branch is converted to s16 for showwaves.
        -- The [ao] branch retains the original audio samples and channel layout.
        local graph = string.format(
            '[aid%d]asplit[ao][wave];[wave]aformat=channel_layouts=stereo:sample_fmts=s16,' ..
            'showwaves=s=%dx%d:mode=p2p:r=20:split_channels=1:colors=white:scale=sqrt:draw=full[vo]',
            aid, width, height)
        local ok = mp.set_property('file-local-options/lavfi-complex', graph)
        if not ok then failed=true; return end
        configured = true
    end
    if mp.get_property_bool('pause') or not mp.get_property_bool('vo-configured') then return end
    local img = mp.command_native({'screenshot-raw', 'video'})
    if not img or type(img.data) ~= 'string' or img.w ~= width or img.h ~= height
        or img.stride < width*4 or #img.data < img.stride*height then return end

    local left, right = {}, {}
    for x=0,width-1 do
        for channel=0,1 do
            local first, last
            for y=channel*height/2,(channel+1)*height/2-1 do
                local offset = y*img.stride+x*4+1
                -- White showwaves pixels have equal B, G, R; one byte suffices.
                if img.data:byte(offset) > 20 then first=first or y; last=y end
            end
            local center = (channel+0.5)*height/2
            local value = first and (center-(first+last)/2)/(height/4) or 0
            if channel == 0 then left[x+1]=value else right[x+1]=value end
        end
    end
    serial = serial+1
    mp.set_property_native('user-data/plexctl-waveform', {
        left=left, right=right, serial=serial, valid=true
    })
end)

mp.set_property_native('user-data/plexctl-waveform-version', 1)
