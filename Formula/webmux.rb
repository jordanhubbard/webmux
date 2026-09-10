class Webmux < Formula
  desc "Browser-based workspace for persistent terminals and remote desktops"
  homepage "https://github.com/jordanhubbard/webmux"
  url "https://github.com/jordanhubbard/webmux/archive/refs/tags/v1.3.10.tar.gz"
  sha256 "b442b784f668864fe00a605c6e7f5d81a15fecc826e38dccbd4fbfd1cda23da5"
  license "BSD-2-Clause"
  head "https://github.com/jordanhubbard/webmux.git", branch: "main"

  depends_on "python@3.14" => :build
  depends_on "node@24"
  depends_on "openssh"

  def install
    ENV.prepend_path "PATH", Formula["node@24"].opt_bin
    ENV["PYTHON"] = Formula["python@3.14"].opt_bin/"python3.14"
    cd "webmux" do
      system "npm", "ci", "--no-audit", "--no-fund"
      system "npm", "run", "build"
      # Reinstall only server dependencies; native modules match the runtime above.
      rm_r "node_modules"
      system "npm", "ci", "--omit=dev", "--workspace=backend", "--no-audit", "--no-fund"
      libexec.install "node_modules", "web", "config.defaults", "package.json", "package-lock.json"
      (libexec/"backend").install "backend/dist", "backend/package.json"
    end
    (bin/"webmux").write <<~SH
      #!/bin/sh
      export WEBMUX_ROOT="#{opt_libexec}"
      export PATH="#{Formula["node@24"].opt_bin}:#{HOMEBREW_PREFIX}/bin:#{HOMEBREW_PREFIX}/sbin:$PATH"
      exec "#{Formula["node@24"].opt_bin}/node" "#{opt_libexec}/backend/dist/index.js" "$@"
    SH
  end

  service do
    run opt_bin/"webmux"
    keep_alive true
    working_dir Dir.home
    log_path var/"log/webmux.log"
    error_log_path var/"log/webmux.log"
  end

  def caveats
    <<~EOS
      Open http://localhost:8080 after starting WebMux.
      Configuration and state stay in ~/.config/webmux (or WEBMUX_HOME).
      Run brew services as your normal user to retain access to your SSH keys.
      Before migrating from a source installation, run make uninstall there.
      Optional tools: brew install mosh tmux guacamole-server
    EOS
  end

  test do
    ENV["WEBMUX_HOME"] = testpath/"state"
    port = free_port
    ENV["HTTP_PORT"] = port.to_s
    pid = spawn (bin/"webmux").to_s, out: (testpath/"server.log").to_s, err: [:child, :out]
    begin
      sleep 1
      health = shell_output("curl --fail --retry 20 --retry-connrefused --retry-delay 1 http://127.0.0.1:#{port}/api/health")
      assert_equal "ok", JSON.parse(health)["status"]
      assert_match '<div id="root">', shell_output("curl --fail http://127.0.0.1:#{port}/")
      assert_path_exists testpath/"state/config/auth.yaml"
      system Formula["node@24"].opt_bin/"node", "-e", <<~JS
        const assert = require('node:assert/strict');
        const argon2 = require('#{libexec}/node_modules/argon2');
        const pty = require('#{libexec}/node_modules/node-pty');
        (async () => {
          assert(await argon2.verify(await argon2.hash('packaging-test'), 'packaging-test'));
          const terminal = pty.spawn('/bin/sh', ['-c', 'printf webmux-pty-ok'], {env: process.env});
          let output = '';
          const timeout = setTimeout(() => process.exit(1), 10000);
          terminal.onData(data => output += data);
          terminal.onExit(({exitCode}) => {
            clearTimeout(timeout);
            assert.equal(exitCode, 0);
            assert.match(output, /webmux-pty-ok/);
          });
        })().catch(error => { console.error(error); process.exit(1); });
      JS
    ensure
      Process.kill "TERM", pid
      Process.wait pid
    end
  end
end
