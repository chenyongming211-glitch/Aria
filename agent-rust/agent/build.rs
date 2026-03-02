fn main() {
    println!("cargo:rerun-if-env-changed=SKIP_EBPF_BUILD");
    
    // 生成 gRPC 代码
    println!("cargo:rerun-if-changed=../proto/aria-agent.proto");
    tonic_build::configure()
        .build_server(false)  // 只生成客户端
        .build_client(true)
        .compile_protos(&["../proto/aria-agent.proto"], &["../proto"])
        .expect("Failed to compile protobuf");
    
    if std::env::var("SKIP_EBPF_BUILD").is_ok() {
        println!("cargo:warning=Skipping eBPF build");
        
        // 创建空的字节码文件以避免编译错误
        let out_dir = std::env::var("OUT_DIR").unwrap();
        let acl_path = std::path::Path::new(&out_dir).join("acl");
        let qos_path = std::path::Path::new(&out_dir).join("qos");
        
        if !acl_path.exists() {
            std::fs::write(&acl_path, &[] as &[u8]).ok();
        }
        if !qos_path.exists() {
            std::fs::write(&qos_path, &[] as &[u8]).ok();
        }
        
        return;
    }
    
    let ebpf_package = aya_build::Package {
        name: "aria-ebpf",
        root_dir: "../ebpf",
        ..Default::default()
    };
    
    aya_build::build_ebpf([ebpf_package], aya_build::Toolchain::default()).unwrap();
}
