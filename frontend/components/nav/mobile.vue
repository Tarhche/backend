<script setup lang="ts">
const cookie = useCookie("tarche")
</script>

<template>
<div class="navbar-backdrop">
  <div class="navbar-content container-sm py-5 px-4 overflow-hidden">
    <ul class="list-unstyled p-0">
      <li>
        <NuxtLink class="nav-link p-2 " :to="cookie ?'/dashboard' : '/auth/login' ">
          <span v-if="cookie">داشبورد</span>
          <span v-else>ورود</span>
        </NuxtLink>
      </li>
    </ul>
  </div>
</div>
</template>

<style scoped>
.navbar-backdrop{
  position: fixed;
  top: 70PX;
  right: 0;
  height: 100%;
  width: 100%;
  z-index: 10;
  background-color: rgba(255 , 255 , 255 , 0.59);
  backdrop-filter: blur(10px);
}
ul li a{
  font-size: 1.2rem;
  font-weight: bolder;
  animation: show 1s forwards ease 1 0.7s ;
  transform: translateX(300px);
  transition: 0.5s;
  position: relative;
}
ul li a:after{
  content:"";
  background-color: #00a2e3;
  position: absolute;
  bottom: 0;
  right: 0;
  height: 3px;
  width: 0;
  transition: 0.5s;
}
ul li:hover a:after{
width: 100%;
}
@keyframes show {
  from{
    transform: translateX(300px);
    opacity: 0;
  }
  to{
    transform: translate(0);
    opacity: 1;
  }
}
</style>