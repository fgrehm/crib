use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize, Debug)]
struct Greeting {
    message: String,
}

fn main() {
    let greeting = Greeting {
        message: "Hello from crib!".to_string(),
    };
    println!("{}", serde_json::to_string_pretty(&greeting).unwrap());
}
