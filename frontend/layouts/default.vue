<template>
    <div>
        <Navbar></Navbar>
        <div id="main">
          <slot />
        </div>
        <Footer></Footer>
    </div>
</template>

<script setup>
useHead({
  titleTemplate:(title) => title ? ` طرح چه | ${ title } ` : "طرح چه"
})
</script>
<style scoped>
#main{
min-height: calc(100vh - 180px);
}
</style>
