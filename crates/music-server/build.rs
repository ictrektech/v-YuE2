fn main() {
    println!("cargo:rerun-if-changed=../../app/public/editor");
    println!("cargo:rerun-if-changed=../../app/examples");
}
