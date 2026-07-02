# PC Build Spec List

**Build date:** July 2026
**Estimated total:** ₹219,608 (as of 02 July 2026, Vedant Computers cart)
**Use case:** Gaming (Elden Ring, Valorant, FromSoft titles) + Development (Fedora 42 / Hyprland daily driver, Go, containers/K8s)

---

## Core Components

| Component | Part | Notes |
|---|---|---|
| **CPU** | AMD Ryzen 7 9800X3D (8C/16T, up to 5.2 GHz, AM5) | Best gaming CPU on AM5; 3D V-Cache; ~162W PPT max |
| **CPU Cooler** | Noctua NH-D15 chromax.black | Well matched to the X3D's thermal profile |
| **Motherboard** | MSI MAG B850 Tomahawk Max WiFi (ATX, AM5) | WiFi 7, 5GbE LAN, PCIe 5.0 x16 + Gen5 M.2, 3× USB-C 10Gbps rear, 20Gbps front header |
| **Memory** | Corsair Vengeance 32GB (2×16GB) DDR5-6000 CL36 | Dual channel — do NOT buy single-stick; enable EXPO in BIOS |
| **Storage** | Adata XPG Gammix S70 Blade 1TB (PCIe Gen4 NVMe) | DRAM cache + TLC; verify revision on arrival |
| **GPU** | ASRock Radeon RX 9070 Challenger 16GB GDDR6 | RDNA 4; 16GB VRAM; 3× DP 2.1a + 1× HDMI 2.1b; 2× 8-pin power |
| **Case** | Lian Li Vector V100 Mid-Tower (White) | ATX; confirm NH-D15 clearance (165mm height) |
| **PSU** | Corsair RM750e — 750W, Cybenetics Gold, Fully Modular, ATX 3.1 | ~57-60% load at absolute max draw (~430-450W); 7-yr warranty |

---

## Power Budget

| Load scenario | Estimated draw | PSU load |
|---|---|---|
| Absolute max (stress test) | ~430-450W | ~57-60% |
| Typical gaming | ~380-420W | ~50-56% |
| Desktop / dev work | ~80-150W | ~11-20% (fanless zone) |

---

## Build Notes & Checklist

- [ ] **RAM:** Install in slots A2 + B2 (2nd and 4th from CPU); enable EXPO for 6000 MT/s
- [ ] **BIOS:** Update to latest before installing the 9800X3D if board ships with early firmware (use Flash BIOS button — no CPU needed)
- [ ] **GPU power:** Use two separate PCIe cables from the PSU, not one daisy-chained cable
- [ ] **SSD:** Install in the top M.2 slot (Gen5-capable, best heatsink); check S70 Blade NAND revision with `smartctl`/CrystalDiskInfo
- [ ] **PSU version:** Confirm the RM750e box says **ATX 3.1** (current revision, not 2022/2023 stock)
- [ ] **Display outputs:** Board has HDMI 2.1 only (no DisplayPort) — irrelevant with the dGPU installed; monitor connects to the RX 9070
- [ ] **Fedora 42:** amdgpu + Mesa work out of the box — no RPM Fusion drivers needed for the GPU

---

## Linux Compatibility (Fedora 42 + Hyprland)

- **GPU:** RX 9070 uses in-kernel `amdgpu` + Mesa RADV — zero-config, Wayland/Hyprland native
- **WiFi 7 / BT 5.4:** MediaTek/Qualcomm module on the Tomahawk supported in recent kernels — verify after first boot
- **5GbE LAN (Realtek 8126):** Supported in-kernel on current Fedora kernels
- **Optional tools:** `lact` or `corectrl` for GPU fan curves; `s-tui`/`zenpower` for CPU monitoring

---

## Upgrade Path

- PSU and board have headroom for a future Ryzen 9 X3D chip (VRM: 14+2+1 80A)
- 2 spare DIMM slots → can go to 64GB later (though 2-DIMM configs clock best on AM5)
- 3 additional M.2 slots free for storage expansion
