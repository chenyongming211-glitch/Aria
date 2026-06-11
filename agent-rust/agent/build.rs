use std::borrow::Cow;
use std::env;
use std::ffi::OsString;
use std::fs;
use std::io::{BufReader, Read};
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};

use cargo_metadata::{Artifact, CompilerMessage, Message, Target};

fn main() {
    if let Err(err) = run() {
        panic!("{err}");
    }
}

fn run() -> Result<(), Box<dyn std::error::Error>> {
    println!("cargo:rerun-if-env-changed=SKIP_EBPF_BUILD");
    println!("cargo:rerun-if-env-changed=CARGO_CFG_TARGET_OS");

    let target_os = env::var("CARGO_CFG_TARGET_OS").unwrap_or_else(|_| "unknown".to_string());
    if target_os != "linux" {
        return Err(format!(
            "aria-agent only supports Linux targets (target_os={target_os}); use GitHub Actions or a Linux builder for agent builds"
        )
        .into());
    }

    // 生成 gRPC 代码
    println!("cargo:rerun-if-changed=../proto/aria_agent.proto");
    tonic_build::configure()
        .build_server(false)
        .build_client(true)
        .compile_protos(&["../proto/aria_agent.proto"], &["../proto"])?;

    if env::var("SKIP_EBPF_BUILD").is_ok() {
        println!("cargo:warning=Skipping eBPF build");

        let out_dir = PathBuf::from(env::var("OUT_DIR")?);
        ensure_placeholder(&out_dir.join("acl"))?;
        ensure_placeholder(&out_dir.join("qos"))?;
        return Ok(());
    }

    build_ebpf_quietly()?;
    Ok(())
}

fn ensure_placeholder(path: &Path) -> Result<(), Box<dyn std::error::Error>> {
    if !path.exists() {
        fs::write(path, &[] as &[u8])?;
    }
    Ok(())
}

fn target_arch_fixup(target_arch: Cow<'_, str>) -> Cow<'_, str> {
    if target_arch.starts_with("riscv64") {
        "riscv64".into()
    } else {
        target_arch
    }
}

fn build_ebpf_quietly() -> Result<(), Box<dyn std::error::Error>> {
    let out_dir = PathBuf::from(env::var_os("OUT_DIR").ok_or("OUT_DIR not set")?);
    let endian = env::var_os("CARGO_CFG_TARGET_ENDIAN").ok_or("CARGO_CFG_TARGET_ENDIAN not set")?;
    let target = if endian == "big" {
        "bpfeb-unknown-none"
    } else if endian == "little" {
        "bpfel-unknown-none"
    } else {
        return Err(format!("unsupported endian={endian:?}").into());
    };

    let target_arch = env::var_os("CARGO_CFG_TARGET_ARCH").ok_or("CARGO_CFG_TARGET_ARCH not set")?;
    let bpf_target_arch = target_arch_fixup(
        target_arch
            .into_string()
            .map_err(|err| format!("invalid target arch: {err:?}"))?
            .into(),
    );

    let ebpf_dir = PathBuf::from("../ebpf");
    println!("cargo:rerun-if-changed={}", ebpf_dir.display());

    let target_dir = out_dir.join("aria-ebpf-target");
    let mut cmd = Command::new("rustup");
    cmd.current_dir(&ebpf_dir);
    cmd.args([
        "run",
        "nightly",
        "cargo",
        "build",
        "--quiet",
        "--package",
        "aria-ebpf",
        "-Z",
        "build-std=core",
        "--bins",
        "--message-format=json-render-diagnostics",
        "--release",
        "--target",
        target,
        "--target-dir",
    ]);
    cmd.arg(&target_dir);

    let mut rustflags = OsString::new();
    const SEPARATOR: &str = "\x1f";
    for flag in [
        "--cfg=bpf_target_arch=\"",
        bpf_target_arch.as_ref(),
        "\"",
        SEPARATOR,
        "-Cdebuginfo=2",
        SEPARATOR,
        "-Clink-arg=--btf",
    ] {
        rustflags.push(flag);
    }
    cmd.env("CARGO_ENCODED_RUSTFLAGS", rustflags);

    for key in ["RUSTC", "RUSTC_WORKSPACE_WRAPPER"] {
        cmd.env_remove(key);
    }

    let mut child = cmd.stdout(Stdio::piped()).stderr(Stdio::piped()).spawn()?;

    let mut stderr_reader = BufReader::new(child.stderr.take().ok_or("missing stderr")?);
    let stderr_handle = std::thread::spawn(move || {
        let mut stderr = String::new();
        let _ = stderr_reader.read_to_string(&mut stderr);
        stderr
    });

    let stdout = BufReader::new(child.stdout.take().ok_or("missing stdout")?);
    let mut executables = Vec::new();

    for message in Message::parse_stream(stdout) {
        match message? {
            Message::CompilerArtifact(Artifact {
                executable,
                target: Target { name, .. },
                ..
            }) => {
                if let Some(executable) = executable {
                    executables.push((name, executable.into_std_path_buf()));
                }
            }
            Message::CompilerMessage(CompilerMessage { message, .. }) => {
                if let Some(rendered) = message.rendered {
                    for line in rendered.lines() {
                        if !line.trim().is_empty() {
                            println!("cargo:warning={line}");
                        }
                    }
                }
            }
            Message::TextLine(_) => {}
            _ => {}
        }
    }

    let status = child.wait()?;
    let stderr = stderr_handle.join().map_err(|_| "failed to join stderr reader")?;

    if !status.success() {
        for line in stderr.lines() {
            if !line.trim().is_empty() {
                println!("cargo:warning={line}");
            }
        }
        return Err(format!("eBPF build failed with status {status}").into());
    }

    for (name, binary) in executables {
        let dst = out_dir.join(name);
        fs::copy(&binary, &dst)?;
    }

    Ok(())
}
