import { computed, ref } from "vue";
export type Role = "viewer" | "operator" | "admin";
export const role = ref<Role>("viewer");
export const canOperate = computed(() => role.value !== "viewer");
export const canAdmin = computed(() => role.value === "admin");
export function setRole(value: Role) {
  role.value = ["viewer", "operator", "admin"].includes(value)
    ? value
    : "viewer";
}
