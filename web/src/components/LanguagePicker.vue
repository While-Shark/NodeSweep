<script setup lang="ts">
import { languages, locale, setLocale, t } from "../i18n";
</script>
<template>
  <label class="language-picker">
    <span>{{ t("语言") }}</span>
    <select
      :value="locale"
      :aria-label="t('语言')"
      @change="setLocale(($event.target as HTMLSelectElement).value)"
    >
      <option
        v-for="language in languages"
        :key="language.value"
        :value="language.value"
        :lang="language.value"
      >
        {{ language.label }}
      </option>
    </select>
  </label>
</template>
