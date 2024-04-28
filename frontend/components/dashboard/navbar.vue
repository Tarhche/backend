<template>
  <nav class="navbar navbar-expand-lg navbar-light">
      <div class="container">
          <NuxtLink class="navbar-brand" to="/">
              <strong>طرح‌چه</strong>
          </NuxtLink>

          <button class="navbar-toggler" type="button" @click="toggle()">
              <span class="navbar-toggler-icon"></span>
          </button>

          <div :class="{'show': value}" class="collapse navbar-collapse">
              <ul class="navbar-nav me-auto mb-2 mb-lg-0">
                  <li class="nav-item">
                      <button class="nav-link" type="button" @click="useAuth().logout()">خروج</button>
                  </li>
              </ul>
          </div>
      </div>
  </nav>
</template>

<script setup>
import { useToggle } from '@vueuse/core'

const [value, toggle] = useToggle()
</script>