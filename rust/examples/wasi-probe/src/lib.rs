// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

//! Probe fixture exercising the host's stubbed-WASI subset
//! end-to-end. The fixture imports a representative set of WASI
//! runtime-init functions, invokes them, and writes their observed
//! responses into a fixed-location output buffer the host reads back
//! via the Wazero memory API.
//!
//! Output layout (40 bytes total, written at offset 0 of plugin
//! memory):
//!
//!   bytes  0..  8  clock_time_get result (u64, little-endian)
//!   bytes  8.. 24  random_get buffer (16 bytes)
//!   bytes 24.. 32  fd_write written-bytes count (u32 at offset 24)
//!                  followed by errno (u32 at offset 28)
//!   bytes 32.. 40  reserved for future probes (zeroed)
//!
//! Under the stubbed subset every byte should be zero EXCEPT:
//!  - bytes 24..28: fd_write reports the length of the iovec it was
//!    handed (the fixture writes 13 bytes of "probe message", so
//!    these 4 bytes are 0x0d 0x00 0x00 0x00).
//!  - bytes 28..32: fd_write errno is 0 (success).
//!
//! The host's integration test reads back the 40 bytes and asserts
//! the expected layout, proving the plugin observed deterministic
//! stub values rather than real host wall-clock, real entropy, or
//! real stdout writes.

const PROBE_OUTPUT_OFFSET: u32 = 0;
const PROBE_OUTPUT_LEN: u32 = 40;

const STDOUT_FD: u32 = 1;

const PROBE_MESSAGE: &[u8] = b"probe message";

#[link(wasm_import_module = "wasi_snapshot_preview1")]
unsafe extern "C" {
    fn clock_time_get(clock_id: u32, precision: u64, time_ptr: u32) -> u32;
    fn random_get(buf: u32, buf_len: u32) -> u32;
    fn fd_write(fd: u32, iovs: u32, iovs_len: u32, written_ptr: u32) -> u32;
}

/// probe_wasi_stubs is the fixture's single export. It exercises
/// three host stubs (clock_time_get, random_get, fd_write) and writes
/// the observed responses into a fixed-location output buffer at
/// offset 0 of plugin linear memory.
///
/// Returns the offset of the output buffer so the host's integration
/// test can read the 40 bytes from there.
#[unsafe(no_mangle)]
pub extern "C" fn probe_wasi_stubs() -> u32 {
    // Zero the output buffer first so any reserved bytes default to 0.
    unsafe {
        let dst = PROBE_OUTPUT_OFFSET as *mut u8;
        for i in 0..PROBE_OUTPUT_LEN {
            *dst.add(i as usize) = 0;
        }
    }

    // 1. Call clock_time_get. The result lands at offset 0 of the
    //    output buffer. clock_id = 0 (CLOCK_REALTIME); precision = 0.
    unsafe {
        let _ = clock_time_get(0, 0, PROBE_OUTPUT_OFFSET);
    }

    // 2. Call random_get for 16 bytes starting at offset 8 of the
    //    output buffer.
    unsafe {
        let _ = random_get(PROBE_OUTPUT_OFFSET + 8, 16);
    }

    // 3. Set up a static iovec describing PROBE_MESSAGE, place it in
    //    plugin memory at a high-enough offset to avoid colliding
    //    with the output buffer, then call fd_write on STDOUT.
    let iovs_offset: u32 = 1024;
    let msg_offset: u32 = 2048;
    let msg_len: u32 = PROBE_MESSAGE.len() as u32;

    unsafe {
        // Copy PROBE_MESSAGE into plugin memory at msg_offset.
        let msg_ptr = msg_offset as *mut u8;
        for (i, b) in PROBE_MESSAGE.iter().enumerate() {
            *msg_ptr.add(i) = *b;
        }

        // Write the iovec { buf: msg_offset, buf_len: msg_len } at
        // iovs_offset (8 bytes total: two u32s little-endian).
        let iov_ptr = iovs_offset as *mut u8;
        let buf_bytes = msg_offset.to_le_bytes();
        let len_bytes = msg_len.to_le_bytes();
        for i in 0..4 {
            *iov_ptr.add(i) = buf_bytes[i];
            *iov_ptr.add(i + 4) = len_bytes[i];
        }

        // Call fd_write with one iovec; written-count lands at
        // PROBE_OUTPUT_OFFSET + 24. fd_write's errno return is
        // captured manually at PROBE_OUTPUT_OFFSET + 28.
        let errno = fd_write(STDOUT_FD, iovs_offset, 1, PROBE_OUTPUT_OFFSET + 24);
        let errno_bytes = errno.to_le_bytes();
        let dst = (PROBE_OUTPUT_OFFSET + 28) as *mut u8;
        for (i, b) in errno_bytes.iter().enumerate() {
            *dst.add(i) = *b;
        }
    }

    PROBE_OUTPUT_OFFSET
}
