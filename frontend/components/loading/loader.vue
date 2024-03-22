

<template>
  <div class="loading">
    <div class="loading-backdrop"><span>طرحچه...</span>
      <div class="loader"></div>
    </div>
  </div>
</template>

<style scoped>

.loading {
  position: fixed;
  background-color: rgba(238, 234, 234, 0.82);
  backdrop-filter: blur(3px);
  width: 100%;
  height: 100%;
  inset: 0;
  display: flex;
  justify-content: center;
  align-items: center;
  color: white;
}

.loading-backdrop {
  width: 150px;
  height: 100px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  gap: 0.8rem;
  background-color: white;
  box-shadow: 0 0 5px 1px rgba(0, 0, 0, 0.3);
  border-radius: 3px;

}

.loading-backdrop > span {
  color: crimson;
  display: inline-block;
  font-weight: bolder;
  font-size: 1.5rem;
  -webkit-text-stroke: 0.5px #b95b7b;
  position: relative;
}
.loader {
  width: 15px;
  aspect-ratio: 1;
  border-radius: 50%;
  animation: l5 1.2s infinite linear alternate;
  opacity: 0.5;
  scale: 0.8;
}
@keyframes l5 {
  0%  {box-shadow: 20px 0 #000, -20px 0 #0002;background: #000 }
  33% {box-shadow: 20px 0 #000, -20px 0 #0002;background: #0002}
  66% {box-shadow: 20px 0 #0002,-20px 0 #000; background: #0002}
  100%{box-shadow: 20px 0 #0002,-20px 0 #000; background: #000 }
}
.loading-backdrop > span::after {
  font-size: 1.5rem;
  content: 'طرحچه...';
  position: absolute;
  right: 0;
  top: 0;
  color: #FFF;
  width: 0;
  height: 100%;
  overflow: hidden;
  box-sizing: border-box;
  animation: animloader 5s ease-in infinite alternate;
}

@keyframes animloader {
  0% {
    width: 0;
  }
  100% {
    width: 100%;
  }
}

</style>