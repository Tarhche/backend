<script setup lang="ts">
import {useRoute} from "vue-router";

const route = useRoute()
const cookie = useCookie("jwt")
const overflow = ref(false)
const navbarMobile = ref(null)
const showNavbarMobile = ref(false)

function navbarActive() {
  navbarMobile.value.classList.toggle('active')
  overflow.value = !overflow.value
  showNavbarMobile.value = !showNavbarMobile.value
}

onMounted(() => {
  watch(overflow, () => {
    if (overflow.value) {
      document.body.classList.add("overflow-hidden")
    } else {
      document.body.classList.remove("overflow-hidden")
    }
  })
})

watch(route , ()=>{
  showNavbarMobile.value=false
  overflow.value=false
})
</script>

<template>
  <nav class="topnav navbar navbar-expand-lg navbar-light bg-white">
    <div class="container">
      <NuxtLink class="navbar-brand" to="/">
        <strong>طرح‌چه</strong>
      </NuxtLink>
      <div class=" p-1 hamburger-btn bg-white border  rounded-3  d-lg-none" ref="navbarMobile"
              type="button" @click="navbarActive">
        <span class="hamburger-icon-top "></span>
        <span class="hamburger-icon-middle "></span>
        <span class="hamburger-icon-bottom "></span>
      </div>
      <div :class="{show: value}" class="navbar-collapse collapse flex-row-reverse" id="navbarColor02" style="">
        <ul class="navbar-nav d-flex align-items-center">
          <li class="nav-item highlight">
            <NuxtLink class="nav-link" :to="cookie ?'/dashboard' : '/auth/login' ">
              <span v-if="cookie">داشبورد</span>
              <span v-else>ورود</span>
            </NuxtLink>
          </li>
        </ul>
      </div>
    </div>
    <transition name="transition" >
      <NavMobile v-if="showNavbarMobile"/>
    </transition>
  </nav>
</template>
<style scoped>
button:active {
  box-shadow: none !important;
  outline: 0;
}

.hamburger-btn {
  width: 40px;
  height: 40px;
  display: flex;
  justify-content: center;
  align-items: center;
  position: relative;
  overflow: hidden;
  border: 1px solid grey;
}

.hamburger-icon-top {
  position: absolute;
  top: 5px;
  height: 5px;
  background-color: #1d2124;
  border-radius: 5px;
  width: 80%;
  transition: 0.5s;

}

.hamburger-icon-bottom {
  position: absolute;
  width: 80%;
  bottom: 5px;
  height: 5px;
  background-color: #1d2124;
  border-radius: 5px;
  transition: 0.5s;

}

.hamburger-icon-middle {
  position: absolute;
  height: 5px;
  background-color: #1d2124;
  border-radius: 5px;
  width: 80%;
  transition: 0.5s;
}

.hamburger-btn.active .hamburger-icon-middle {
  opacity: 0;
  transform: translate(20px);
  transition: 0.8s;
}

.hamburger-btn.active .hamburger-icon-bottom {
  rotate: -45deg;
  transform: translate(2.5px, 4px);
  transform-origin: left;
  width: 100%;
  transition: rotate 0.8s;
  bottom: 4px;

}

.hamburger-btn.active .hamburger-icon-top {
  top: 4px;
  rotate: 45deg;
  transform: translate(2.5px, -4px);
  transform-origin: left;
  width: 100%;
  transition: rotate ease 0.8s;
}

.active {
  border: 3px solid gray !important;
  box-shadow: none;
  outline: 0;
}

.transition-enter-active {
  transition: all 0.7s ease;
}

.transition-leave-active {
  transition: all 0.7s ease;
}

.transition-enter-from, .transition-leave-to {
  opacity: 0;
  transform: translatex(100%);
}

.transition-enter-to, .transition-leave-from {
  opacity: 1;
}
</style>