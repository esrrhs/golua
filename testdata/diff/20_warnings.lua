-- the warning system is a three-state machine and starts off: '@on'
-- turns it on, a message can be split over several calls, '@off' ends
-- it again, and control messages are never printed.
warn("hidden 1")
warn("@on", "a", "b")
warn("c", "d")
warn("@off")
warn("hidden 2")
warn("@on")
warn("", "@on") -- no control: a real (empty) warning
warn("Z", "Z", "Z")
warn("@on")
warn("last")
