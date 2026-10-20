-- A loopback round trip: connect, send and receive, time out, select,
-- and UDP.  Every port here is whatever the kernel hands out, so two of
-- these can run at once and neither needs the network.
local socket = require "socket"
math.randomseed(0)

local server = assert(socket.bind("127.0.0.1", 0))
local ip, port = server:getsockname()
server:settimeout(2)

-- connect, send a line, receive it on the other side
local c = assert(socket.tcp())
assert(c:settimeout(2))
assert(c:connect(ip, port))
local s = assert(server:accept())
assert(s:settimeout(2))
c:send("hello\n")
print("line:", s:receive())

-- a chunk big enough to need several write()s and read()s
c:send(string.rep("x", 100000))
print("big chunk:", #s:receive(100000))
c:close()
s:close()

-- receive("*a") with nothing coming: it must time out, not block
local c2 = assert(socket.tcp())
assert(c2:connect(ip, port))
local s2 = assert(server:accept())
s2:settimeout(0.3)
local t0 = socket.gettime()
local ok, err = s2:receive("*a")
local dt = socket.gettime() - t0
print("timed out:", ok == nil, tostring(err), "took at least 0.25s:", dt >= 0.25)
c2:close()
s2:close()

-- select: an idle listener has nothing to read
local r = socket.select({server}, nil, 0.1)
print("select idle:", #r)

-- UDP, to itself
local u = assert(socket.udp())
u:setsockname("127.0.0.1", 0)
local _, uport = u:getsockname()
u:setpeername("127.0.0.1", uport)
assert(u:send("ping"))
print("udp:", u:receive())
u:close()

server:close()
print("done")
