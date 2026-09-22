class Webmux < Formula
  desc "Browser-based workspace for persistent terminals and remote desktops"
  homepage "https://github.com/jordanhubbard/webmux"
  url "https://github.com/jordanhubbard/webmux/archive/refs/tags/v1.3.14.tar.gz"
  sha256 "db36b72bc6b1b2278e88c98e7db7bf1882a6f043949b546493213fb24a446eca"
  license "BSD-2-Clause"
  head "https://github.com/jordanhubbard/webmux.git", branch: "main"

  depends_on "go" => :build
  depends_on "node@24" => :build
  depends_on "openssh"

  def install
    ENV.prepend_path "PATH", Formula["node@24"].opt_bin
    odie "Native builds require a source revision containing the Go migration." unless (buildpath/"scripts/package-native.mts").exist?
    cd "webmux" do
      system "npm", "ci", "--workspace=frontend", "--include-workspace-root", "--no-audit", "--no-fund"
      system "npm", "run", "build", "--workspace=frontend"
    end
    system "node", "scripts/package-native.mts", buildpath/"native-dist"
    archives = Dir[buildpath/"native-dist/*-native.tar.gz"]
    odie "Expected exactly one native runtime archive" unless archives.length == 1
    (buildpath/"native-stage").mkpath
    system "tar", "-xzf", archives.first, "-C", buildpath/"native-stage"
    roots = Dir[buildpath/"native-stage/webmux-*"]
    odie "Expected exactly one extracted runtime" unless roots.length == 1
    libexec.install Dir["#{roots.first}/*"]
    launch = "exec \"#{opt_libexec}/bin/webmux\" \"$@\""
    runtime_path = "#{HOMEBREW_PREFIX}/bin:#{HOMEBREW_PREFIX}/sbin"
    (bin/"webmux").write <<~SH
      #!/bin/sh
      export WEBMUX_ROOT="#{opt_libexec}"
      export PATH="#{runtime_path}:$PATH"
      #{launch}
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
      require "net/http"
      require "securerandom"
      credentials = JSON.generate(username: "formula-test", password: SecureRandom.hex(24))
      %w[bootstrap login].each do |action|
        response = Net::HTTP.post(URI("http://127.0.0.1:#{port}/api/auth/#{action}"),
                                 credentials, "Content-Type" => "application/json")
        assert_kind_of Net::HTTPSuccess, response
        assert_kind_of String, JSON.parse(response.body)["token"]
      end
      assert_path_exists libexec/"bin/webmux"
      refute_path_exists libexec/"node_modules"
    ensure
      Process.kill "TERM", pid
      Process.wait pid
    end
  end
end
