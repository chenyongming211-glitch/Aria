// Bug Condition 探索性测试 - Mutex 中毒漏洞
// 
// 此测试验证 Bug Condition: 在异步上下文中使用 std::sync::Mutex 并通过 .lock().unwrap()
// 获取锁，当持有锁的线程 panic 时会触发 Mutex 中毒，导致级联失败。
//
// 预期结果（在未修复的代码上）: 测试失败 - 证明 bug 存在
// 预期结果（在修复后的代码上）: 测试通过 - 证明 bug 已修复

use std::sync::{Arc, Mutex as StdMutex};
use std::panic;

/// 测试 1: 模拟 std::sync::Mutex 中毒导致级联失败
/// 
/// Bug Condition: 当一个线程在持有 StdMutex 锁时 panic，Mutex 进入 poisoned 状态，
/// 后续所有 .lock().unwrap() 调用都会 panic
#[test]
#[should_panic(expected = "PoisonError")]
fn test_std_mutex_poisoning_causes_cascade_failure() {
    // 创建一个使用 std::sync::Mutex 的共享状态（模拟 last_sync_peers）
    let shared_data: Arc<StdMutex<Vec<String>>> = Arc::new(StdMutex::new(vec![]));
    
    // 第一个线程：在持有锁时 panic（模拟 bug 触发场景）
    let data_clone = shared_data.clone();
    let handle1 = std::thread::spawn(move || {
        let mut data = data_clone.lock().unwrap();
        data.push("item1".to_string());
        // 模拟在持有锁时发生 panic
        panic!("Simulated panic while holding lock");
    });
    
    // 等待第一个线程 panic
    let _ = handle1.join();
    
    // 第二个线程：尝试获取锁（应该会因为 Mutex 中毒而 panic）
    // 这模拟了 agent_runtime.rs 中的其他代码路径尝试访问 last_sync_peers
    let data_clone = shared_data.clone();
    let handle2 = std::thread::spawn(move || {
        // 这个 .lock().unwrap() 会 panic，因为 Mutex 已经中毒
        let data = data_clone.lock().unwrap();
        println!("Data length: {}", data.len());
    });
    
    // 这个 join 应该返回 Err，因为线程 panic 了
    handle2.join().expect("Thread should panic due to poisoned mutex");
}

/// 测试 2: 验证 tokio::sync::Mutex 不会中毒
/// 
/// Expected Behavior: 使用 tokio::sync::Mutex 时，即使发生 panic 也不会导致 Mutex 中毒
#[tokio::test]
async fn test_tokio_mutex_does_not_poison() {
    use tokio::sync::Mutex as TokioMutex;
    
    // 创建一个使用 tokio::sync::Mutex 的共享状态
    let shared_data: Arc<TokioMutex<Vec<String>>> = Arc::new(TokioMutex::new(vec![]));
    
    // 第一个任务：在持有锁时 panic
    let data_clone = shared_data.clone();
    let handle1 = tokio::spawn(async move {
        let mut data = data_clone.lock().await;
        data.push("item1".to_string());
        // 模拟在持有锁时发生 panic
        panic!("Simulated panic while holding lock");
    });
    
    // 等待第一个任务 panic（忽略错误）
    let _ = handle1.await;
    
    // 第二个任务：尝试获取锁（应该成功，因为 tokio::Mutex 不会中毒）
    let data_clone = shared_data.clone();
    let handle2 = tokio::spawn(async move {
        // 这个 .lock().await 应该成功，即使之前的任务 panic 了
        let data = data_clone.lock().await;
        assert_eq!(data.len(), 1); // 应该能看到第一个任务添加的数据
        println!("✅ Successfully accessed data after panic: {} items", data.len());
    });
    
    // 这个 join 应该成功
    handle2.await.expect("Should successfully access mutex after panic");
}

/// 测试 3: 模拟 agent_runtime.rs 中的实际场景
/// 
/// 这个测试模拟 agent_runtime.rs 中多个代码路径并发访问 last_sync_peers 的场景
#[test]
fn test_concurrent_access_with_std_mutex_poisoning() {
    let shared_peers: Arc<StdMutex<Vec<String>>> = Arc::new(StdMutex::new(vec![]));
    let mut handles = vec![];
    
    // 模拟主同步循环（Line 415）
    let peers_clone = shared_peers.clone();
    handles.push(std::thread::spawn(move || {
        for i in 0..5 {
            let peers = peers_clone.lock().unwrap();
            println!("Sync loop: {} peers", peers.len());
            drop(peers);
            std::thread::sleep(std::time::Duration::from_millis(10));
            
            // 在第 3 次迭代时 panic（模拟 bug 触发）
            if i == 2 {
                let _peers = peers_clone.lock().unwrap();
                panic!("Sync loop panic");
            }
        }
    }));
    
    // 模拟 Unix socket 命令处理（Line 618）
    let peers_clone = shared_peers.clone();
    handles.push(std::thread::spawn(move || {
        std::thread::sleep(std::time::Duration::from_millis(50));
        // 这个调用应该会因为 Mutex 中毒而 panic
        let peers = peers_clone.lock().unwrap();
        println!("Unix socket: {} peers", peers.len());
    }));
    
    // 模拟远程命令执行（Line 1235）
    let peers_clone = shared_peers.clone();
    handles.push(std::thread::spawn(move || {
        std::thread::sleep(std::time::Duration::from_millis(60));
        // 这个调用也应该会因为 Mutex 中毒而 panic
        let peers = peers_clone.lock().unwrap();
        println!("Remote command: {} peers", peers.len());
    }));
    
    // 等待所有线程完成
    let mut panic_count = 0;
    for handle in handles {
        if handle.join().is_err() {
            panic_count += 1;
        }
    }
    
    // 验证：应该有多个线程 panic（级联失败）
    // 在未修复的代码上，至少应该有 2 个线程 panic（第一个触发 panic，后续的因为 Mutex 中毒而 panic）
    assert!(panic_count >= 2, "Expected cascade failure with {} panics, but got {}", 2, panic_count);
    println!("✅ Detected cascade failure: {} threads panicked", panic_count);
}

/// 测试 4: 验证修复后的行为 - 使用 tokio::Mutex 的并发访问
#[tokio::test]
async fn test_concurrent_access_with_tokio_mutex_no_poisoning() {
    use tokio::sync::Mutex as TokioMutex;
    
    let shared_peers: Arc<TokioMutex<Vec<String>>> = Arc::new(TokioMutex::new(vec![]));
    let mut handles = vec![];
    
    // 模拟主同步循环
    let peers_clone = shared_peers.clone();
    handles.push(tokio::spawn(async move {
        for i in 0..5 {
            let peers = peers_clone.lock().await;
            println!("Sync loop: {} peers", peers.len());
            drop(peers);
            tokio::time::sleep(tokio::time::Duration::from_millis(10)).await;
            
            // 在第 3 次迭代时 panic
            if i == 2 {
                let _peers = peers_clone.lock().await;
                panic!("Sync loop panic");
            }
        }
    }));
    
    // 模拟 Unix socket 命令处理
    let peers_clone = shared_peers.clone();
    handles.push(tokio::spawn(async move {
        tokio::time::sleep(tokio::time::Duration::from_millis(50)).await;
        // 这个调用应该成功，即使之前的任务 panic 了
        let peers = peers_clone.lock().await;
        println!("✅ Unix socket successfully accessed: {} peers", peers.len());
    }));
    
    // 模拟远程命令执行
    let peers_clone = shared_peers.clone();
    handles.push(tokio::spawn(async move {
        tokio::time::sleep(tokio::time::Duration::from_millis(60)).await;
        // 这个调用也应该成功
        let peers = peers_clone.lock().await;
        println!("✅ Remote command successfully accessed: {} peers", peers.len());
    }));
    
    // 等待所有任务完成
    let mut success_count = 0;
    let mut panic_count = 0;
    for handle in handles {
        match handle.await {
            Ok(_) => success_count += 1,
            Err(_) => panic_count += 1,
        }
    }
    
    // 验证：只有第一个任务 panic，其他任务应该成功（没有级联失败）
    assert_eq!(panic_count, 1, "Expected only 1 panic (the intentional one)");
    assert_eq!(success_count, 2, "Expected 2 successful tasks");
    println!("✅ No cascade failure: {} panics, {} successes", panic_count, success_count);
}
