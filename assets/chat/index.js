(() => {
  // expectingMessage is set to true
  // if the user has just submitted a message
  // and so we should scroll the next message into view when received.
  let expectingMessage = false;
  let ws;
  function dial() {
    console.log(location.host);
    // generate a random number between 10 and 99
    const clientId = Math.floor(Math.random() * 900) + 100;

    const conn = new WebSocket(
      `ws://${location.host}/subscribe?id=6ec0bd7f-11c0-43da-975e-2a8ad9eba${clientId}&lat=51.5073509&lon=-0.1277581999999997`,
      clientId > 500 ? "map-client" : "map-taxi",
    );
    ws = conn;

    conn.addEventListener("close", (ev) => {
      appendLog(
        `WebSocket Disconnected code: ${ev.code}, reason: ${ev.reason}`,
        true,
      );
      if (ev.code !== 1001) {
        appendLog("Reconnecting in 1s", true);
        setTimeout(dial, 1000);
      }
    });
    conn.addEventListener("open", (ev) => {
      console.info("websocket connected");
    });

    // This is where we handle messages received.
    conn.addEventListener("message", (ev) => {
      if (typeof ev.data !== "string") {
        console.error("unexpected message type", typeof ev.data);
        return;
      }
      const p = appendLog(ev.data);
      if (expectingMessage) {
        p.scrollIntoView();
        expectingMessage = false;
      }
    });
  }
  dial();

  const messageLog = document.getElementById("message-log");
  const publishForm = document.getElementById("publish-form");
  const messageInput = document.getElementById("message-input");

  // appendLog appends the passed text to messageLog.
  function appendLog(text, error) {
    const p = document.createElement("p");
    // Adding a timestamp to each message makes the log easier to read.
    p.innerText = `${new Date().toLocaleTimeString()}: ${text}`;
    if (error) {
      p.style.color = "red";
      p.style.fontStyle = "bold";
    }
    messageLog.append(p);
    return p;
  }
  appendLog("Submit a message to get started!");

  // onsubmit publishes the message from the user when the form is submitted.
  publishForm.onsubmit = async (ev) => {
    console.log(ws);
    ev.preventDefault();

    const msg = messageInput.value;
    if (msg === "") {
      return;
    }
    messageInput.value = "";

    expectingMessage = true;
    ws.send(msg);
  };
})();
