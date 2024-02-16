<script setup lang="ts">
import { useToggle } from '@vueuse/core'

const [value, toggle] = useToggle()
</script>

<template>
    <nav class="topnav navbar navbar-expand-lg navbar-light bg-white">
        <div class="container">
            <NuxtLink class="navbar-brand" to="/">
                <strong>طرح‌چه</strong>
            </NuxtLink>
            <button class="navbar-toggler collapsed" type="button" @click="toggle()">
                <span class="navbar-toggler-icon"></span>
            </button>
            <div :class="{show: value}" class="navbar-collapse collapse flex-row-reverse" id="navbarColor02" style="">
                <ul class="navbar-nav d-flex align-items-center">
                    <li class="nav-item highlight">
                        <NuxtLink class="nav-link" to="/auth/login">ورود</NuxtLink>
                    </li>
                </ul>
            </div>
        </div>
    </nav>
</template>