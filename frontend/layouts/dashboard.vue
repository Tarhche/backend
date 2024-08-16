<template>
  <div>
    <dashboardNavbar/>
    <slot/>
    <Footer/>
  </div>
</template>

<script setup>
useHead({
  titleTemplate: (title) => title ? `داشبورد | ${title}` : "داشبورد"
})
</script>
