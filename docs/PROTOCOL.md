# KinoBox serial protocol

## Link layer

| Setting      | Value                                                       |
|--------------|-------------------------------------------------------------|
| Port         | Serial / USB-serial port (COM1…COM20 on Windows)            |
| Baud         | 115200                                                      |
| Framing      | 8N1, no flow control                                        |
| Modem lines  | **DTR must be asserted** after opening the port             |
| Host → box   | **nothing** – the protocol has no commands                  |

The box streams fixed-size binary frames on its own. There is no header, no
sync byte, no checksum and no length field.

## Frame: 28 bytes = 14 × 16-bit words, little-endian

| Word | Bytes | Meaning                              | Conversion                          |
|------|-------|--------------------------------------|-------------------------------------|
| 0    | 0–1   | Load (consumer/battery) current, ADC | `I_load = w0 · BattMaxA / ADCmax`   |
| 1    | 2–3   | Battery voltage, ADC                 | `U = w1 · MaxV / ADCmax`            |
| 2–11 | 4–23  | Generator current, channel 0…9, ADC  | `I_k = w · UserMaxA / ADCmax`       |
| 12   | 24–25 | Status word (see below)              | bit fields                          |
| 13   | 26–27 | Temperature                          | `T[°C] = w13 / 100`                 |

Words 0–11 are ADC readings in the range 0…ADCmax; word 13 is unsigned.

### Scaling parameters

| Parameter  | Default | Meaning                                  |
|------------|---------|------------------------------------------|
| `ADCBits`  | 10      | ADC resolution; `ADCmax = 2^ADCBits − 1` = 1023 |
| `MaxV`     | 20 V    | Battery voltage at full scale            |
| `BattMaxA` | 150 A   | Load current at full scale               |
| `UserMaxA` | 50 A    | Generator channel current at full scale  |

### Derived values

```
P_load   = U · I_load                    # consumed power, W
P_k      = U · I_k                       # per-rider power, W
P_gen    = Σ P_k                         # total generated power, W
E_gen   += P_gen · Δt_ms / 1000 / 3.6e6  # generated energy, kWh
E_net   += (P_gen − P_load) · Δt_ms / 1000 / 3.6e6
I_gen_counts = Σ w2..w11                 # basis of the over-current check
```

The reference host polls one frame every 500 ms and uses Δt = 500 ms.

### Channel → rider mapping

The frame carries 10 channel slots. On the 8-bike KinoBox slot 4 is unused:
riders 1–4 = slots 0–3, riders 5–8 = slots 5–8. For other rider counts the
mapping is 1:1.

### Status word (word 12)

| Bits  | Field                                  | Values                           |
|-------|----------------------------------------|----------------------------------|
| 15–14 | Low voltage (inverter under-voltage)   | 0 = OK, 1 = warning, 2 = tripped |
| 13–12 | High voltage (input over-voltage)      | 0/1/2                            |
| 11–10 | High temperature                       | 0/1/2                            |
|  9–8  | High current (input over-current)      | 0/1/2                            |
|  7–0  | Protection countdown, seconds          | non-zero while a trip is latched |

The box enforces protection itself: it switches the inputs or the inverter off
and re-enables them after the countdown (default 10 s). The host only
displays the state – conventionally yellow for any warning (1) and red for
any trip (2).

### Protection thresholds (informational)

Thresholds in raw ADC counts (with the default scaling) and their physical
equivalents:

| Protection                    | Warning / Trip (counts)            | Physical         |
|-------------------------------|------------------------------------|------------------|
| Input over-voltage            | 716 / 767                          | 14.0 V / 15.0 V  |
| Inverter under-voltage        | 588 / 563                          | 11.5 V / 11.0 V  |
| Input over-current            | 1637 / 2046 (Σ of channel counts)  | 80 A / 100 A     |
| Over-temperature              | –                                  | 70 °C / 80 °C    |

## Host implementation notes

* Read exactly 28 bytes per frame and decode only complete frames; on a
  short read keep the previous values. After about 5 consecutive missed
  frames, close and reopen the port.
* No framing: after opening, discard input (or wait for a gap) before
  reading. If the box sends faster than you read, frames queue up in the OS
  buffer – prefer reading continuously and keeping the latest frame. A
  plausibility check (U within 0…MaxV, each 2-bit status field ≤ 2) is a
  cheap resync heuristic.
* Integrate energy using real elapsed time rather than a fixed 500 ms.
