import { createApp } from "vue";
import { dialog } from "./dialog";
import App from "./App.vue";
import "./style.css";
createApp(App).directive("dialog", dialog).mount("#app");
