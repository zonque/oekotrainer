# KinoBox serial protocol (reverse-engineered)

Source: `Oekosoft_Kinobox_v2.exe` (build 2021.07.21, wxWidgets 3.0.2, MinGW,
serial I/O via the `ctb` library). Addresses below refer to that binary
(image base 0x400000).

## Link layer

| Setting      | Value                                                       |
|--------------|-------------------------------------------------------------|
| Port         | `\\.\COMn`, n = 1..20 (registry `Port Wahl`, default `COM 3`); on connection loss the app probes all 20 ports (0x43df40) and picks the next one that opens (0x43df90) |
| Baud         | 115200 (registry `Baudrate`)                                |
| Framing      | 8N1, no flow control                                        |
| Modem lines  | **DTR is asserted** right after opening (`SetLineState(DTR)`) |
| Host → box   | **nothing** – the PC software never calls `Write()`         |

The box streams fixed-size binary frames on its own. There is no header, no
sync byte, no checksum and no length field.

## Frame: 28 bytes = 14 × uint16, little-endian

The app reads exactly 28 bytes per timer tick (`Read(buf, 0x1c)` at 0x43e1b9);
a read returning anything other than 28 is counted as a miss ("Nichts da: %d").
After `Count Read Data` (default 5) consecutive misses it disconnects and
reconnects ("Zuviele Nichts - Neuverbinden"). The 28 bytes are copied verbatim
and decoded by 0x422ec0:

| Word | Bytes | Meaning                              | Conversion                          |
|------|-------|--------------------------------------|-------------------------------------|
| 0    | 0–1   | Load (consumer/battery) current, ADC | `I_load = w0 · BattMaxA / ADCmax`   |
| 1    | 2–3   | Battery voltage, ADC                 | `U = w1 · MaxV / ADCmax`            |
| 2–11 | 4–23  | Generator current, channel 0…9, ADC  | `I_k = w · UserMaxA / ADCmax`       |
| 12   | 24–25 | Status word (see below)              | bit fields                          |
| 13   | 26–27 | Temperature                          | `T[°C] = w13 / 100`                 |

Words 0–11 are read as *signed* `int16`, word 13 as unsigned (Ghidra
decompilation of 0x422ec0); in practice the ADC values are 0…1023.
A frame is decoded only after a successful 28-byte read; after a miss the
previous values are kept.

`ADCmax = 2^ADCBits − 1`. Defaults: `ADC Bitbreite` = 10 → 1023,
`Max V` = 20 V, `Batt Max A` = 150 A, `User Max A` = 50 A.

Derived values computed by the PC:

```
P_load   = U · I_load                    # "Verbraucht" W
P_k      = U · I_k                       # per-rider W
P_gen    = Σ P_k
E_gen   += P_gen · T_ms / 1000 / 3.6e6   # kWh, "Erzeugt"
E_net   += (P_gen − P_load) · T_ms / 1000 / 3.6e6
I_gen_counts = Σ w2..w11                 # used for over-current check
```

`T_ms` is the fixed polling period `Timer Update KB` (default 500 ms); the
software does **not** timestamp frames, it assumes one frame per tick.

### Channel → rider mapping

10 channel slots exist (`Max User` = 10). With `Count User` = 8 (the shipped
8-bike KinoBox) slot 4 is skipped: riders 1–4 = slots 0–3, riders 5–8 =
slots 5–8 (getter 0x423060). For other user counts the mapping is 1:1.

### Status word (word 12)

| Bits  | Field            | Values                                   |
|-------|------------------|------------------------------------------|
| 15–14 | UV state (inverter under-voltage) | 0 = OK, 1 = warning, 2 = tripped |
| 13–12 | OV state (input over-voltage)     | 0/1/2                          |
| 11–10 | OT state (over-temperature)       | 0/1/2                          |
|  9–8  | OC state (input over-current)     | 0/1/2                          |
|  7–0  | Protection countdown, seconds (`%d s`) | non-zero while a trip is latched |

The PC shows yellow for any "1", red for any "2" (0x42f7a0). The box itself
enforces protection (it switches inputs / inverter off and re-enables after
`Protect Time` = 10 s); the PC only displays. The field-to-bit assignment is
confirmed by the status-icon renderer (0x42e600), which draws the bytes split
out by 0x42f7a0 with the icons `LowVolt` (bits 15–14), `HighVolt` (13–12),
`HighTemp` (11–10) and `HighCur` (9–8).

### Thresholds (registry defaults, informational)

These live in the PC config, are in raw ADC counts, and match the manual:

| Key                        | Counts | Physical |
|----------------------------|--------|----------|
| Protect OVP MF ADC On/Off  | 716 / 767   | 14.0 V / 15.0 V |
| Protect UVP WR ADC On/Off  | 588 / 563   | 11.5 V / 11.0 V |
| Protect OCP MF ADC On/Off  | 1637 / 2046 (Σ of channel counts) | 80 A / 100 A |
| Protect OTP MF On/Off      | 70 / 80 °C  |  |

## Caveats for a reimplementation

* No framing: after opening, discard input (or wait for a gap) and read 28
  bytes at a time. If the box sends faster than you read, frames queue up
  in the OS buffer — prefer reading continuously and keeping the latest frame.
  A plausibility check (U within 0..MaxV, status high nibbles ≤ 2) is a cheap
  resync heuristic.
* Integrate energy using real elapsed time rather than a fixed 500 ms.
