Microsoft.Windows.Console.ConPTY 1.24.260710001, published by Microsoft.Terminal.

Source: https://www.nuget.org/packages/Microsoft.Windows.Console.ConPTY/1.24.260710001
Package: https://api.nuget.org/v3-flatcontainer/microsoft.windows.console.conpty/1.24.260710001/microsoft.windows.console.conpty.1.24.260710001.nupkg
SHA-256: 175640566a3b59c4b132070ee96c2c77e5ab7edd2e92732a5eb3610bbf63d90e
License: MIT; see LICENSE.

The Windows executable embeds the immutable package, verifies its digest, and
extracts its architecture's conpty.dll and compatible native OpenConsole hosts to
the service user's cache. Existing files must match the embedded bytes before
loading. Loading uses the absolute DLL path and restricts dependency search to
its directory and Windows system libraries. Linux/macOS builds omit the package.

The system ConPTY on Windows Server 2022 consumes alternate-screen switches,
which prevents correct terminal replay. Use the same redistributable runtime on
all supported Windows hosts so terminal behavior does not depend on the OS image.
