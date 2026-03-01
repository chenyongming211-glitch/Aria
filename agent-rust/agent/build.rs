fn main() {
    println!("cargo:rerun-if-env-changed=SKIP_EBPF_BUILD");
    
    if std::env::var("SKIP_EBPF_BUILD").is_ok() {
        println!("cargo:warning=Skipping eBPF build");
        return;
    }
    
    let ebpf_package = aya_build::Package {
        name: "aria-ebpf",
        root_dir: "../ebpf",
        ..Default::default()
    };
    
    aya_build::build_ebpf([ebpf_package], aya_build::Toolchain::default()).unwrap();
}
