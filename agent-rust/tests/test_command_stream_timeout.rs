use std::time::Duration;

use aria_agent::grpc_client::{command_stream_rpc_timeout, unary_rpc_timeout};

fn main() {
    assert_eq!(unary_rpc_timeout(), Some(Duration::from_secs(30)));
    assert_eq!(command_stream_rpc_timeout(), None);
}
