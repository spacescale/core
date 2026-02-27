use std::{
    env, fs,
    path::{Path, PathBuf},
};

/// collect_protos walks a directory tree and gathers every `.proto` file path.
///
/// High-level behavior:
/// - Start at `dir`.
/// - Read all entries in that directory.
/// - If an entry is a subdirectory, recurse into it.
/// - If an entry is a file ending in `.proto`, push its path into `out`.
///
/// Why `out: &mut Vec<PathBuf>` instead of returning `Vec<PathBuf>`:
/// - All recursive calls share one output buffer.
/// - Each call appends into the same list.
/// - This avoids repeatedly creating/merging vectors at each recursion level.
///
/// Return value:
/// - `Ok(())` means traversal completed successfully.
/// - `Err(std::io::Error)` is returned immediately on the first filesystem error.
///
/// Note on `?` in this function:
/// - `fs::read_dir(dir)?` fails fast if the directory cannot be read.
/// - `entry?` fails fast if reading an individual directory entry fails.
/// - `collect_protos(&path, out)?` propagates errors from recursive calls.
fn collect_protos(dir: &Path, out: &mut Vec<PathBuf>) -> Result<(), std::io::Error> {
    for entry in fs::read_dir(dir)? {
        let path = entry?.path();
        if path.is_dir() {
            // Recurse into subdirectories; use the same `out` list.
            collect_protos(&path, out)?;
        } else if path.extension().and_then(|e| e.to_str()) == Some("proto") {
            // This file has a `.proto` extension, so keep it.
            out.push(path);
        }
    }
    Ok(())
}

fn main() {}
