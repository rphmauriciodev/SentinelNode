#ifndef WATCHDOG_SYSTEM_H
#define WATCHDOG_SYSTEM_H

#include <Arduino.h>

class WatchdogSystem {
public:
    // Timeout in seconds. Default is 10s.
    static void init(uint32_t timeoutSeconds = 10);
    
    // Feeds the watchdog timer for the current running task (loop)
    static void feed();
};

#endif // WATCHDOG_SYSTEM_H
