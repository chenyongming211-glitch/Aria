fn main() {
    println!("cargo:rerun-if-env-changed=SKIP_EBPF_BUILD");

    if std::env::var("SKIP_EBPF_BUILD").is_ok() {
        println!("cargo:warning=Skipping eBPF build");
        return;
    }

    // 确保 rust-src 组件已安装（用于 build-std）
    let output = std::process::Command::new("rustup")
        .args(["component", "list", "--toolchain", "nightly"])
        .output()
        .expect("Failed to list rustup components");
    let installed = String::from_utf8_lossy(&output.stdout);
    if !installed.contains("rust-src") {
        println!("cargo:warning=rust-src component not found, installing...");
        let status = std::process::Command::new("rustup")
            .args(["component", "add", "rust-src", "--toolchain", "nightly"])
            .status()
            .expect("Failed to install rust-src");
        if !status.success() {
            panic!("Failed to install rust-src component");
        }
    }

    // 合并 RUSTFLAGS 确保 panic=abort
    let mut rustflags = String::from("-C panic=abort");
    if let Ok(existing) = std::env::var("RUSTFLAGS") {
        rustflags = format!("{} {}", existing, rustflags);
    }

    let target = "bpfel-unknown-none";

    // 编译 eBPF 程序
    let status = std::process::Command::new("cargo")
        .args([
            "+nightly",
            "build",
            "-Z",
            "build-std=core",
            "-Z",
            "build-std-features=panic_immediate_abort",
            "--release",
            "--target",
            target,
            "--manifest-path",
            "../ebpf/Cargo.toml",
        ])
        .env("RUSTFLAGS", rustflags)
        .env("RUSTUP_TOOLCHAIN", "nightly")
        .status()
        .expect("Failed to build eBPF programs");

    if !status.success() {
        panic!("Failed to build eBPF programs");
    }

    // 复制生成的 eBPF 文件到 OUT_DIR
    let ebpf_target_dir = std::path::Path::new("../target")
        .join(target)
        .join("release");

    let out_dir = std::env::var("OUT_DIR").unwrap();
    let dest_path = std::path::Path::new(&out_dir);

    for prog in ["qos", "acl"] {
        let src = ebpf_target_dir.join(prog);
        let dst = dest_path.join(prog);
        if let Err(e) = std::fs::copy(&src, &dst) {
            panic!("Failed to copy {}: {}", prog, e);
        }
        println!("cargo:rerun-if-changed={}", src.display());
    }

    println!("cargo:rustc-env=OUT_DIR={}", out_dir);
}
