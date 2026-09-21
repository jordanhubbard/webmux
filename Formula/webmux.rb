class Webmux < Formula
  desc "Browser-based workspace for persistent terminals and remote desktops"
  homepage "https://github.com/jordanhubbard/webmux"
  url "https://github.com/jordanhubbard/webmux/archive/refs/tags/v1.3.10.tar.gz"
  sha256 "b442b784f668864fe00a605c6e7f5d81a15fecc826e38dccbd4fbfd1cda23da5"
  license "BSD-2-Clause"
  head "https://github.com/jordanhubbard/webmux.git", branch: "main"

  option "with-native-server", "Build Go from a stable source containing the migration"
  head do
    depends_on "go" => :build
    depends_on "node@24" => :build
  end
  stable do
    if build.with? "native-server"
      depends_on "go" => :build
      depends_on "node@24" => :build
    else
      depends_on "python@3.14" => :build
      depends_on "node@24"
    end
  end
  depends_on "openssh"

  def install
    ENV.prepend_path "PATH", Formula["node@24"].opt_bin
    if build.head? || build.with?("native-server")
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
    else
      ENV["PYTHON"] = Formula["python@3.14"].opt_bin/"python3.14"
      cd "webmux" do
        system "npm", "ci", "--no-audit", "--no-fund"
        legacy_build = (buildpath/"webmux/scripts/native.mts").exist? ? "build:node" : "build"
        system "npm", "run", legacy_build
        rm_r "node_modules"
        system "npm", "ci", "--omit=dev", "--workspace=backend", "--no-audit", "--no-fund"
        libexec.install "node_modules", "web", "config.defaults", "package.json", "package-lock.json"
        (libexec/"backend").install "backend/dist", "backend/package.json"
      end
      helper = buildpath/"scripts/verify-node-runtime.mts"
      (libexec/"scripts").install helper if helper.exist?
      launch = "exec \"#{Formula["node@24"].opt_bin}/node\" \"#{opt_libexec}/backend/dist/index.js\" \"$@\""
      runtime_path = "#{Formula["node@24"].opt_bin}:#{HOMEBREW_PREFIX}/bin:#{HOMEBREW_PREFIX}/sbin"
    end
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
      if (libexec/"bin/webmux").exist?
        refute_path_exists libexec/"node_modules"
      elsif (libexec/"scripts/verify-node-runtime.mts").exist?
        system Formula["node@24"].opt_bin/"node", libexec/"scripts/verify-node-runtime.mts", libexec
      end
    ensure
      Process.kill "TERM", pid
      Process.wait pid
    end
  end
end
